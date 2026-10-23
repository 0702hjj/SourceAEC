// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var branchName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_/-]{0,79}$`)

func validateBranch(branch string) error {
	if !branchName.MatchString(branch) || strings.Contains(branch, "//") || strings.HasSuffix(branch, "/") {
		return errors.New("invalid branch name")
	}
	return nil
}
func git(repo string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	argv := append([]string{"-C", repo, "-c", "user.name=SourceAEC", "-c", "user.email=sourceaec@localhost", "-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1"}
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Git operation failed: %s", strings.TrimSpace(string(data)))
	}
	return strings.TrimSpace(string(data)), nil
}
func workspace(db queryer, root, model string) (*candidate, error) {
	directory, err := os.MkdirTemp("", "sourceaec-git-")
	if err != nil {
		return nil, err
	}
	c := &candidate{directory: directory, repo: filepath.Join(directory, "repo"), branch: "main", root: root, model: model}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(directory)
		}
	}()
	if err = os.Mkdir(c.repo, 0700); err != nil {
		return nil, err
	}
	if _, err = git(c.repo, "init", "-b", "main"); err != nil {
		return nil, err
	}
	var bundle []byte
	err = db.QueryRow("SELECT bundle,branch FROM repositories WHERE model_id=$1", model).Scan(&bundle, &c.branch)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		path := filepath.Join(directory, "history.bundle")
		if err = os.WriteFile(path, bundle, 0600); err != nil {
			return nil, err
		}
		if _, err = git(c.repo, "fetch", path, "+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*", "+refs/notes/*:refs/notes/*", "--update-head-ok"); err != nil {
			return nil, err
		}
		if _, err = git(c.repo, "checkout", c.branch); err != nil {
			return nil, err
		}
	}
	success = true
	return c, nil
}
func persist(db queryer, c *candidate) error {
	path := filepath.Join(c.directory, "saved.bundle")
	_ = os.Remove(path)
	if _, err := git(c.repo, "bundle", "create", path, "--all"); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO repositories(model_id,bundle,branch) VALUES($1,$2,$3) ON CONFLICT(model_id) DO UPDATE SET bundle=excluded.bundle,branch=excluded.branch", c.model, data, c.branch)
	return err
}

type gitArgs struct {
	Model     string         `json:"model"`
	Script    string         `json:"script"`
	Note      string         `json:"note"`
	Source    string         `json:"source"`
	Inputs    map[string]any `json:"inputs"`
	Version   string         `json:"version"`
	Action    string         `json:"action"`
	Branch    string         `json:"branch"`
	Candidate string         `json:"candidate"`
}

func commitGit(db queryer, c *candidate, a gitArgs) (map[string]string, error) {
	if err := os.WriteFile(filepath.Join(c.repo, "build.py"), []byte(a.Script), 0600); err != nil {
		return nil, err
	}
	identity := map[string]any{}
	if build, ok := a.Inputs["current.build.json"].(map[string]any); ok {
		identity, _ = build["identity"].(map[string]any)
	}
	environment := map[string]any{}
	for _, key := range []string{"python", "packages", "environment", "helpers", "engine"} {
		if value, ok := identity[key]; ok {
			environment[key] = value
		}
	}
	inputs := a.Inputs["current.inputs.json"]
	if inputs == nil {
		inputs = map[string]any{}
	}
	data, err := json.MarshalIndent(map[string]any{"current.inputs.json": inputs, "current.build.json": map[string]any{"identity": environment}}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(c.repo, "inputs.json"), data, 0600); err != nil {
		return nil, err
	}
	if _, err = git(c.repo, "add", "--", "build.py", "inputs.json"); err != nil {
		return nil, err
	}
	note := a.Note
	if note == "" {
		note = "Update model"
	}
	source := a.Source
	if source == "" {
		source = "human"
	}
	if _, err = git(c.repo, "commit", "--allow-empty", "-m", note, "-m", "SourceAEC-Source: "+source); err != nil {
		return nil, err
	}
	revision, err := git(c.repo, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	if len(a.Inputs) > 0 {
		data, err = json.Marshal(a.Inputs)
		if err != nil {
			return nil, err
		}
		if _, err = git(c.repo, "notes", "--ref=build", "add", "-f", "-m", string(data), revision); err != nil {
			return nil, err
		}
	}
	if a.Version != "" {
		if !regexp.MustCompile(`^v[0-9]+$`).MatchString(a.Version) {
			return nil, errors.New("invalid version tag")
		}
		if _, err = git(c.repo, "tag", a.Version); err != nil {
			return nil, err
		}
	}
	if err = persist(db, c); err != nil {
		return nil, err
	}
	return map[string]string{"commit": revision, "branch": c.branch}, nil
}
func gitState(c *candidate) (map[string]any, error) {
	refs, err := git(c.repo, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := []string{}
	commits := []map[string]any{}
	result := map[string]any{"branch": c.branch, "branches": branches, "commits": commits}
	if refs == "" {
		return result, nil
	}
	result["branches"] = strings.Split(refs, "\n")
	raw, err := git(c.repo, "log", "--branches", "--tags", "--topo-order", "-100", "--format=%H%x09%P%x09%s")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			return nil, errors.New("invalid Git log")
		}
		commits = append(commits, map[string]any{"commit": parts[0], "parents": strings.Fields(parts[1]), "note": parts[2]})
	}
	result["commits"] = commits
	result["commit"], err = git(c.repo, "rev-parse", "HEAD")
	return result, err
}
func (r *Runtime) gitOperation(req Request) (any, error) {
	if req.Op == "git.validate" {
		var a gitArgs
		if err := decode(req.Args, &a); err != nil {
			return nil, err
		}
		return nil, validateBranch(a.Branch)
	}
	db, err := r.database(req)
	if err != nil {
		return nil, err
	}
	var a gitArgs
	if err = decode(req.Args, &a); err != nil {
		return nil, err
	}

	if req.Op == "git.persist" || req.Op == "git.discard" || req.Op == "git.inputs" {
		c := r.candidates[a.Candidate]
		if c == nil || c.root != req.Root || c.model != a.Model {
			return nil, errors.New("invalid Git candidate")
		}
		if req.Op == "git.discard" {
			delete(r.candidates, a.Candidate)
			return nil, os.RemoveAll(c.directory)
		}
		if req.Op == "git.inputs" {
			return candidateInputs(c)
		}
		return nil, persist(db, c)
	}
	if req.Op == "git.import" {
		return nil, importHistory(db, req.Root, a.Model)
	}
	c, err := workspace(db, req.Root, a.Model)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(c.directory)
		}
	}()
	switch req.Op {
	case "git.commit":
		return commitGit(db, c, a)
	case "git.state":
		return gitState(c)
	case "git.prepare":
		if err = validateBranch(a.Branch); err != nil {
			return nil, err
		}
		switch a.Action {
		case "create":
			_, err = git(c.repo, "checkout", "-b", a.Branch)
			c.branch = a.Branch
		case "checkout":
			_, err = git(c.repo, "checkout", a.Branch)
			c.branch = a.Branch
		case "merge":
			_, err = git(c.repo, "merge", "--no-ff", "--no-edit", a.Branch)
		default:
			return nil, errors.New("unknown version action")
		}
		if err != nil {
			return nil, err
		}
		script, e := os.ReadFile(filepath.Join(c.repo, "build.py"))
		if e != nil {
			return nil, e
		}
		id := token()
		r.candidates[id] = c
		keep = true
		return map[string]string{"candidate": id, "repo": c.repo, "branch": c.branch, "script": string(script)}, nil
	default:
		return nil, errors.New("unknown Git operation")
	}
}
