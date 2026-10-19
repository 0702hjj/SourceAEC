// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import "testing"

func TestSharedParameterBindingAndScalarValidation(t *testing.T) {
	r := New()
	defer r.Close()
	facts := []callFact{{Argument: "height", Path: []string{"wall", "height"}}, {Argument: "name", Literal: true, Value: "Wall"}, {Argument: "key", Literal: true, Value: "wall"}, {Argument: "dynamic"}}
	args := bindingArgs{Params: map[string]any{"wall": map[string]any{"height": 3.0}}, Facts: facts}
	got := invoke(t, r, "edit.bindings", "", "", args).([]binding)
	if len(got) != 2 || string(canonical(got[0].Value)) != "3" || got[1].Argument != "name" {
		t.Fatal(got)
	}
	args.Argument = "height"
	args.Value = 4.0
	params := invoke(t, r, "edit.parameter", "", "", args).(map[string]any)
	if string(canonical(params["wall"].(map[string]any)["height"])) != "4" {
		t.Fatal(params)
	}
	args.Argument = "name"
	if _, err := r.Handle(request("edit.parameter", "", "", args)); err == nil {
		t.Fatal("accepted unbound literal")
	}
	args.Argument = "height"
	args.Value = []int{1}
	if _, err := r.Handle(request("edit.parameter", "", "", args)); err == nil {
		t.Fatal("accepted non-scalar")
	}
}

func TestVersionChangeRequiresCleanCurrentRevision(t *testing.T) {
	r := New()
	defer r.Close()
	args := map[string]any{"script": "a=1", "revision": digestBytes([]byte("a=1")), "staged": 0}
	invoke(t, r, "edit.clean", "", "", args)
	args["staged"] = 1
	if _, err := r.Handle(request("edit.clean", "", "", args)); err == nil {
		t.Fatal("accepted unsaved changes")
	}
	args["staged"] = 0
	args["revision"] = "stale"
	if _, err := r.Handle(request("edit.clean", "", "", args)); err == nil {
		t.Fatal("accepted stale revision")
	}
}
