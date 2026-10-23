// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

func digestBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func fileDigest(path string) any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return digestBytes(data)
}
func canonical(value any) []byte { data, _ := json.Marshal(value); return data }
func collectInputs(root, model string) (map[string]string, error) {
	result := map[string]string{}
	path, err := safePath(root, "models/"+model+"/model.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	var metadata struct {
		Project string `json:"projectId"`
	}
	if err = json.Unmarshal(data, &metadata); err != nil {
		return result, nil
	}
	if !regexp.MustCompile(`^p_[a-zA-Z0-9]+$`).MatchString(metadata.Project) {
		return result, nil
	}
	var total int64
	for _, name := range []string{"plans", "skill-work"} {
		base, err := safePath(root, name+"/"+metadata.Project)
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(base, func(path string, d fs.DirEntry, e error) error {
			if os.IsNotExist(e) {
				return nil
			}
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if d.Name() == "plan_history" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".json" && ext != ".py" && ext != ".dxf" {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			total += info.Size()
			if total > 256<<20 {
				return errors.New("build inputs exceed 256 MiB")
			}
			if info.Size() > 64<<20 {
				return errors.New("build input exceeds 64 MiB")
			}
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			key, e := filepath.Rel(root, path)
			if e != nil {
				return e
			}
			result[filepath.ToSlash(key)] = base64.StdEncoding.EncodeToString(data)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
func projectInputs(workdir string, inputs map[string]string) (string, error) {
	target := filepath.Join(workdir, ".build-inputs")
	if err := os.Mkdir(target, 0700); err != nil {
		return "", err
	}
	for key, encoded := range inputs {
		path, err := safePath(target, key)
		if err != nil {
			return "", errors.New("invalid build input path")
		}
		data, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil {
			return "", err
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", err
		}
		if err = os.WriteFile(path, data, 0444); err != nil {
			return "", err
		}
	}
	return target, nil
}

type buildArgs struct {
	Model           string            `json:"model"`
	Workdir         string            `json:"workdir"`
	Inputs          map[string]string `json:"inputs"`
	Script          string            `json:"script"`
	Product         string            `json:"product"`
	Map             string            `json:"map"`
	EnvDir          string            `json:"envDir"`
	Python          string            `json:"python"`
	Dependencies    []string          `json:"dependencies"`
	Helpers         []string          `json:"helpers"`
	Engine          string            `json:"engine"`
	ProductName     string            `json:"productName"`
	Stats           any               `json:"stats"`
	CodeFingerprint string            `json:"codeFingerprint"`
	TaskRuntime     string            `json:"taskRuntime"`
}

func (r *Runtime) buildOperation(req Request) (any, error) {
	var a buildArgs
	if err := decode(req.Args, &a); err != nil {
		return nil, err
	}
	switch req.Op {
	case "build.collect":
		return collectInputs(req.Root, a.Model)
	case "build.project":
		return projectInputs(a.Workdir, a.Inputs)
	case "build.fingerprint":
		return digestBytes(canonical(a.Inputs)), nil
	case "build.identity":
		return buildIdentity(a)
	case "build.reusable":
		return reusableBuild(a)
	case "build.publish":
		return nil, r.publishBuild(req, a)
	case "build.cache.prepare":
		return prepareCache(a)
	case "build.cache.publish":
		return publishCache(a)
	default:
		return nil, errors.New("unknown build operation")
	}
}
