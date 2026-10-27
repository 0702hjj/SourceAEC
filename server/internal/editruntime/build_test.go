// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"os"
	"path/filepath"
	"testing"

	"ifcviewer/server/internal/artifact"
)

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func TestCollectBuildInputsFiltersHistoryAndSymlinks(t *testing.T) {
	r, root := databaseRuntime(t)
	put(t, filepath.Join(root, "models/model/model.json"), `{"projectId":"p_synthetic"}`)
	put(t, filepath.Join(root, "plans/p_synthetic/plan.json"), `{}`)
	put(t, filepath.Join(root, "plans/p_synthetic/plan_history/v1.json"), `{}`)
	put(t, filepath.Join(root, "skill-work/p_synthetic/drawing.dxf"), "synthetic")
	put(t, filepath.Join(root, "skill-work/p_synthetic/ignored.bin"), "ignored")
	if err := os.Symlink(filepath.Join(root, "plans/p_synthetic/plan.json"), filepath.Join(root, "plans/p_synthetic/link.json")); err != nil {
		t.Fatal(err)
	}
	if err := artifact.CaptureTree(root); err != nil {
		t.Fatal(err)
	}
	inputs := invoke(t, r, "build.collect", root, "", buildArgs{Model: "model"}).(map[string]string)
	if len(inputs) != 2 {
		t.Fatal(inputs)
	}
	if err := artifact.WriteFile(filepath.Join(root, "models/model/model.json"), []byte(`{"projectId":"p_../escape"}`), 0644); err != nil {
		t.Fatal(err)
	}
	inputs = invoke(t, r, "build.collect", root, "", buildArgs{Model: "model"}).(map[string]string)
	if len(inputs) != 0 {
		t.Fatal(inputs)
	}
}
func TestSuccessfulBuildReuseRequiresProductMapAndIdentity(t *testing.T) {
	r := New()
	defer r.Close()
	root := t.TempDir()
	product, mapPath := filepath.Join(root, "model.ifc"), filepath.Join(root, "map.json")
	put(t, product, "IFC")
	put(t, mapPath, `{}`)
	a := buildArgs{Script: "build()", Product: product, Map: mapPath, EnvDir: filepath.Join(root, "env"), Python: "3.10", Helpers: []string{filepath.Join(root, "helpers")}, Inputs: map[string]string{}}
	if err := r.publishBuild(Request{}, a); err != nil {
		t.Fatal(err)
	}
	if reused, err := reusableBuild(a); err != nil || !reused {
		t.Fatal(reused, err)
	}
	put(t, product, "changed")
	if reused, _ := reusableBuild(a); reused {
		t.Fatal("reused changed product")
	}
	put(t, product, "IFC")
	put(t, filepath.Join(root, "helpers/helper.py"), "changed helper")
	if reused, _ := reusableBuild(a); reused {
		t.Fatal("reused changed helper")
	}
}
