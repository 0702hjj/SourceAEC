// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
		db, err := r.database(req)
		if err != nil {
			return nil, err
		}
		return collectInputs(runtimeContext(req), db, a.Model)
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
