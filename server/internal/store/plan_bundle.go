// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"ifcviewer/server/internal/editruntime"
)

// PutBundle persists a related set of plan artifacts in one transaction.
func (s *PlanStore) PutBundle(ctx context.Context, projectID string, contents map[string][]byte) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validProjectID(projectID) {
		return nil, ErrInvalidID
	}
	names := []string{}
	for name, data := range contents {
		if !validPlanName(name) {
			return nil, ErrInvalidKind
		}
		if !json.Valid(data) {
			return nil, ErrInvalidJSON
		}
		names = append(names, name)
	}
	sort.Strings(names)
	root, err := filepath.Abs(s.DataDir)
	if err != nil {
		return nil, err
	}
	runtime := editruntime.NewEmbedded()
	defer runtime.Close()
	tx := ""
	call := func(op string, args any) (any, error) {
		raw, e := json.Marshal(args)
		if e != nil {
			return nil, e
		}
		return runtime.Handle(editruntime.Request{Context: ctx, Op: op, Root: root, Tx: tx, Args: raw})
	}
	value, err := call("begin", nil)
	if err != nil {
		return nil, err
	}
	tx = value.(string)
	defer func() { _, _ = call("rollback", nil) }()
	if _, err = call("lock", "plan:"+projectID); err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	versions := map[string]string{}
	for _, name := range names {
		current := filepath.Join(root, "plans", projectID, name)
		version := "v1"
		old, e := call("read", filepath.ToSlash(filepath.Join("plans", projectID, name)))
		if e == nil {
			historyDir, e := filepath.Abs(s.historyDir(projectID, name))
			if e != nil {
				return nil, e
			}
			historical := nextHistoryVersion(historyDir)
			files[filepath.Join(historyDir, historical+".json")] = old.([]byte)
			version = "v" + itoa(atoiOrZero(historical[1:])+1)
		} else if !os.IsNotExist(e) {
			return nil, e
		}
		files[current] = contents[name]
		versions[name] = version
	}
	entries := []map[string]any{}
	for path, data := range files {
		key, e := filepath.Rel(root, path)
		if e != nil {
			return nil, e
		}
		entries = append(entries, map[string]any{"path": filepath.ToSlash(key), "content": data})
	}
	if _, err = call("publish", entries); err != nil {
		return nil, err
	}
	for path, data := range files {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err = os.WriteFile(path, data, 0644); err != nil {
			return nil, err
		}
	}
	if _, err = call("commit", nil); err != nil {
		return nil, err
	}
	return versions, nil
}
