// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import "testing"

func gitCall(t *testing.T, r *Runtime, root, op, tx string, args gitArgs) any {
	t.Helper()
	args.Model = "m_0000000000000001"
	return invoke(t, r, "git."+op, root, tx, args)
}
func changeBranch(t *testing.T, r *Runtime, root, action, branch string) {
	t.Helper()
	result := gitCall(t, r, root, "prepare", "", gitArgs{Action: action, Branch: branch}).(map[string]string)
	gitCall(t, r, root, "persist", "", gitArgs{Candidate: result["candidate"]})
	gitCall(t, r, root, "discard", "", gitArgs{Candidate: result["candidate"]})
}
func TestGitBranchesConflictAndTransactionRollback(t *testing.T) {
	r, root := databaseRuntime(t)
	first := gitCall(t, r, root, "commit", "", gitArgs{Script: "a=1\nb=1\n", Version: "v1"}).(map[string]string)
	changeBranch(t, r, root, "create", "feature")
	gitCall(t, r, root, "commit", "", gitArgs{Script: "a=2\nb=1\n"})
	changeBranch(t, r, root, "checkout", "main")
	changeBranch(t, r, root, "merge", "feature")
	state := gitCall(t, r, root, "state", "", gitArgs{}).(map[string]any)
	commits := state["commits"].([]map[string]any)
	if state["branch"] != "main" || len(commits[0]["parents"].([]string)) != 2 {
		t.Fatal(state)
	}
	found := false
	for _, commit := range commits {
		if commit["commit"] == first["commit"] {
			found = true
		}
	}
	if !found {
		t.Fatal("missing historical commit")
	}
	tx := invoke(t, r, "begin", root, "", nil).(string)
	gitCall(t, r, root, "commit", tx, gitArgs{Script: "a=9\n"})
	invoke(t, r, "rollback", root, tx, nil)
	restored := gitCall(t, r, root, "state", "", gitArgs{}).(map[string]any)
	if restored["commit"] != state["commit"] {
		t.Fatal("Git head survived rollback")
	}
	changeBranch(t, r, root, "create", "conflict")
	gitCall(t, r, root, "commit", "", gitArgs{Script: "a=3\n"})
	changeBranch(t, r, root, "checkout", "main")
	expected := gitCall(t, r, root, "commit", "", gitArgs{Script: "a=4\n"}).(map[string]string)
	_, err := r.Handle(request("git.prepare", root, "", gitArgs{Model: "m_0000000000000001", Action: "merge", Branch: "conflict"}))
	if err == nil {
		t.Fatal("expected merge conflict")
	}
	restored = gitCall(t, r, root, "state", "", gitArgs{}).(map[string]any)
	if restored["commit"] != expected["commit"] {
		t.Fatal("conflict changed head")
	}
}
func TestGitDerivedMetadataDoesNotConflict(t *testing.T) {
	r, root := databaseRuntime(t)
	source := "# first\na=1\n\n\n\n\n\n\n# last\nb=1\n"
	inputs := func(hash string) map[string]any {
		return map[string]any{"current.build.json": map[string]any{"productHash": hash}}
	}
	gitCall(t, r, root, "commit", "", gitArgs{Script: source, Inputs: inputs("first")})
	changeBranch(t, r, root, "create", "feature")
	gitCall(t, r, root, "commit", "", gitArgs{Script: "# first\na=2\n\n\n\n\n\n\n# last\nb=1\n", Inputs: inputs("feature")})
	changeBranch(t, r, root, "checkout", "main")
	gitCall(t, r, root, "commit", "", gitArgs{Script: "# first\na=1\n\n\n\n\n\n\n# last\nb=2\n", Inputs: inputs("main")})
	changeBranch(t, r, root, "merge", "feature")
	state := gitCall(t, r, root, "state", "", gitArgs{}).(map[string]any)
	if len(state["commits"].([]map[string]any)) != 4 {
		t.Fatal(state)
	}
}
