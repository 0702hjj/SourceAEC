// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

func pythonFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !d.IsDir() && filepath.Ext(path) == ".py" {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}
func copyFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0600)
}
func cacheFiles(directory string, maxTotal int64) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		return nil, err
	}
	var total int64
	for _, path := range files {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > 16<<20 {
			return nil, errors.New("invalid task cache file")
		}
		total += info.Size()
		if maxTotal > 0 && total > maxTotal {
			return nil, errors.New("task cache exceeds 64 MiB")
		}
	}
	return files, nil
}
func prepareCache(a buildArgs) (map[string]string, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	target := filepath.Join(a.Workdir, "sourceaec-runtime")
	if err = copyFile(binary, target); err != nil {
		return nil, err
	}
	if err = os.Chmod(target, 0500); err != nil {
		return nil, err
	}

	if err := copyFile(a.TaskRuntime, filepath.Join(a.Workdir, "sourceaec_tasks.py")); err != nil {
		return nil, err
	}
	incoming, outgoing := filepath.Join(a.Workdir, ".task-input"), filepath.Join(a.Workdir, ".task-output")
	if err := os.Mkdir(incoming, 0700); err != nil {
		return nil, err
	}
	files, err := cacheFiles(a.Product+".tasks_cache", 64<<20)
	if err == nil {
		for _, path := range files {
			if err = copyFile(path, filepath.Join(incoming, filepath.Base(path))); err != nil {
				return nil, err
			}
		}
	}
	digest := sha256.New()
	digest.Write([]byte(a.CodeFingerprint))
	data, err := os.ReadFile(a.TaskRuntime)
	if err != nil {
		return nil, err
	}
	digest.Write(data)
	digest.Write(canonical(a.Inputs))
	digest.Write(canonical(runtimeRevision()))
	for _, root := range a.Helpers {
		files, err := pythonFiles(root)
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			rel, _ := filepath.Rel(root, path)
			digest.Write([]byte(rel))
			data, err = os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			digest.Write(data)
		}
	}
	digest.Write([]byte(a.EnvDir))
	paths, err := filepath.Glob(filepath.Join(a.EnvDir, "lib/python*/site-packages/*.dist-info/METADATA"))
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		digest.Write(data)
	}
	return map[string]string{"SOURCEAEC_TASK_INPUT": incoming, "SOURCEAEC_TASK_OUTPUT": outgoing, "SOURCEAEC_BUILD_FINGERPRINT": hex.EncodeToString(digest.Sum(nil))}, nil
}
func publishCache(a buildArgs) (any, error) {
	source := filepath.Join(a.Workdir, ".task-output")
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return nil, nil
	}
	files, err := cacheFiles(source, 64<<20)
	if err != nil {
		return nil, nil
	}
	target, temporary := a.Product+".tasks_cache", a.Product+".tasks_cache.tmp"
	if err = os.RemoveAll(temporary); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(temporary, 0700); err != nil {
		return nil, err
	}
	for _, path := range files {
		if err = copyFile(path, filepath.Join(temporary, filepath.Base(path))); err != nil {
			return nil, err
		}
	}
	if err = os.RemoveAll(target); err != nil {
		return nil, err
	}
	if err = os.Rename(temporary, target); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(target, "stats.json"))
	if err != nil {
		return nil, nil
	}
	var stats any
	if json.Unmarshal(data, &stats) != nil {
		return nil, nil
	}
	return stats, nil
}
