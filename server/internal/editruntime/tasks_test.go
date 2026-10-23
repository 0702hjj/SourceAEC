// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTaskIncrementalReuseCorruptionAndDependencyOutputs(t *testing.T) {
	root := t.TempDir()
	request := taskRequest{Params: map[string]any{"height": 3, "width": 10}, Tasks: []taskSpec{{Key: "wall", Params: []string{"height"}}, {Key: "window", Dependencies: []string{"wall"}}, {Key: "slab", Params: []string{"width"}}}, Incoming: filepath.Join(root, "in"), Outgoing: filepath.Join(root, "out"), Fingerprint: "code-and-environment"}
	calls := []string{}
	compute := func(key string, params, deps map[string]any) (any, error) {
		calls = append(calls, key)
		return map[string]any{"params": params, "dependencies": deps}, nil
	}
	baseline, err := evaluateTasks(request, compute)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(request.Outgoing, request.Incoming); err != nil {
		t.Fatal(err)
	}
	request.Params["height"] = 4
	calls = nil
	updated, err := evaluateTasks(request, compute)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"wall", "window"}) || string(canonical(updated["slab"])) != string(canonical(baseline["slab"])) {
		t.Fatal(calls, updated)
	}
	os.RemoveAll(request.Incoming)
	os.Rename(request.Outgoing, request.Incoming)
	cache := filepath.Join(request.Incoming, digestBytes([]byte("slab"))+".json")
	os.WriteFile(cache, []byte(`{"signature":"corrupt"}`), 0600)
	calls = nil
	if _, err = evaluateTasks(request, compute); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"slab"}) {
		t.Fatal(calls)
	}
}
func TestTaskCycleAndDuplicateFail(t *testing.T) {
	req := taskRequest{Tasks: []taskSpec{{Key: "a", Dependencies: []string{"a"}}}, Outgoing: t.TempDir()}
	_, err := evaluateTasks(req, nil)
	if err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatal(err)
	}
	req.Tasks = append(req.Tasks, req.Tasks[0])
	_, err = evaluateTasks(req, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatal(err)
	}
}
