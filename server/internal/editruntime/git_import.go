// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

func importHistory(db queryer, root, model string) error {
	var count int
	if err := db.QueryRow("SELECT count(*) FROM repositories WHERE model_id=$1", model).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	paths, err := filepath.Glob(filepath.Join(root, "models", model, "scripts", "v*.py"))
	if err != nil {
		return err
	}
	pattern := regexp.MustCompile(`^v([0-9]+)\.py$`)
	numbers := map[int]string{}
	for _, path := range paths {
		match := pattern.FindStringSubmatch(filepath.Base(path))
		if match != nil {
			n, e := strconv.Atoi(match[1])
			if e != nil {
				return e
			}
			numbers[n] = path
		}
	}
	keys := []int{}
	for n := range numbers {
		keys = append(keys, n)
	}
	sort.Ints(keys)
	for _, n := range keys {
		data, err := os.ReadFile(numbers[n])
		if err != nil {
			return err
		}
		c, err := workspace(db, root, model)
		if err != nil {
			return err
		}
		version := "v" + strconv.Itoa(n)
		_, err = commitGit(db, c, gitArgs{Script: string(data), Note: "Import " + version, Source: "migration", Version: version})
		os.RemoveAll(c.directory)
		if err != nil {
			return err
		}
	}
	return nil
}
