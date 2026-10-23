// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

func buildIdentity(a buildArgs) (map[string]any, error) {
	packages := map[string]string{}
	paths, err := filepath.Glob(filepath.Join(a.EnvDir, "lib/python*/site-packages/*.dist-info/METADATA"))
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		name, version := "", ""
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "Name: ") {
				name = strings.TrimPrefix(line, "Name: ")
			}
			if strings.HasPrefix(line, "Version: ") {
				version = strings.TrimPrefix(line, "Version: ")
			}
			if name != "" && version != "" {
				break
			}
		}
		if name != "" && version != "" {
			packages[name] = version
		}
	}
	helpers := map[string]any{}
	for i, root := range a.Helpers {
		files, err := pythonFiles(root)
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			rel, _ := filepath.Rel(root, path)
			helpers[strconv.Itoa(i)+"/"+filepath.ToSlash(rel)] = fileDigest(path)
		}
	}
	engine := map[string]any{}
	for _, name := range []string{"runner.py", "backend.py", "task_runtime.py", "build_cache.py", "build_inputs.py", "deps.py", "build_record.py"} {
		engine[name] = fileDigest(filepath.Join(a.Engine, name))
	}
	// The trusted Go implementation itself participates in cache invalidation.
	engine["goRuntime"] = runtimeRevision()
	dependencies := a.Dependencies
	if dependencies == nil {
		dependencies = []string{}
	}
	return map[string]any{"scriptHash": digestBytes([]byte(a.Script)), "python": a.Python, "packages": packages, "helpers": helpers, "product": a.ProductName, "engine": engine, "environment": filepath.Base(a.EnvDir), "dependencies": dependencies, "inputsHash": digestBytes(canonical(a.Inputs))}, nil
}

var revisionOnce sync.Once
var binaryRevision any

func runtimeRevision() any {
	revisionOnce.Do(func() {
		if path, err := os.Executable(); err == nil {
			binaryRevision = fileDigest(path)
		}
	})
	return binaryRevision
}

func reusableBuild(a buildArgs) (bool, error) {
	data, err := os.ReadFile(a.Product + ".build.json")
	if err != nil {
		return false, nil
	}
	var record map[string]any
	if json.Unmarshal(data, &record) != nil {
		return false, nil
	}
	identity, err := buildIdentity(a)
	if err != nil {
		return false, err
	}
	return string(canonical(record["identity"])) == string(canonical(identity)) && record["productHash"] == fileDigest(a.Product) && record["mapHash"] == fileDigest(a.Map), nil
}
func (r *Runtime) publishBuild(req Request, a buildArgs) error {
	identity, err := buildIdentity(a)
	if err != nil {
		return err
	}
	record := map[string]any{"identity": identity, "productHash": fileDigest(a.Product), "mapHash": fileDigest(a.Map), "tasks": a.Stats}
	record["revision"] = digestBytes(canonical(record))
	content := canonical(record)
	path := a.Product + ".build.json"
	if req.Root != "" {
		key, err := filepath.Rel(req.Root, path)
		if err != nil {
			return err
		}
		db, err := r.database(req)
		if err != nil {
			return err
		}
		if err = r.publish(req, db, []fileEntry{{Path: filepath.ToSlash(key), Content: content}}); err != nil {
			return err
		}
	}
	temporary := path + ".tmp"
	if err = os.WriteFile(temporary, content, 0644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
