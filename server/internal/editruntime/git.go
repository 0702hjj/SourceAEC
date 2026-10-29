// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"ifcviewer/server/internal/versionstore"
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
	return gitContext(context.Background(), repo, args...)
}
func (c *candidate) git(args ...string) (string, error) { return gitContext(c.ctx, c.repo, args...) }
func gitContext(parent context.Context, repo string, args ...string) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	argv := append([]string{"-C", repo, "-c", "user.name=SourceAEC", "-c", "user.email=sourceaec@localhost", "-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1"}
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Git operation failed: %w: %s", err, strings.TrimSpace(string(data)))
	}
	return strings.TrimSpace(string(data)), nil
}
func workspaceSnapshot(ctx context.Context, root, model string, snapshot RepositorySnapshot) (*candidate, error) {
	directory, err := os.MkdirTemp("", "sourceaec-git-")
	if err != nil {
		return nil, err
	}
	c := &candidate{directory: directory, repo: filepath.Join(directory, "repo"), branch: "main", root: root, model: model, ctx: ctx}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(directory)
		}
	}()
	if err = os.Mkdir(c.repo, 0700); err != nil {
		return nil, err
	}
	if _, err = c.git("init", "-b", "main"); err != nil {
		return nil, err
	}
	if snapshot.Bundle != nil {
		c.branch = snapshot.Branch
		bundle := snapshot.Bundle
		path := filepath.Join(directory, "history.bundle")
		if err = os.WriteFile(path, bundle, 0600); err != nil {
			return nil, err
		}
		if _, err = c.git("fetch", path, "+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*", "+refs/notes/*:refs/notes/*", "--update-head-ok"); err != nil {
			return nil, err
		}
		if _, err = c.git("checkout", c.branch); err != nil {
			return nil, err
		}
	}
	success = true
	return c, nil
}
func persist(db queryer, c *candidate) error {
	path := filepath.Join(c.directory, "saved.bundle")
	_ = os.Remove(path)
	if _, err := c.git("bundle", "create", path, "--all"); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var head string
	if !c.projectionOnly {
		head, err = c.git("rev-parse", "HEAD")
		if err != nil {
			return err
		}
	}
	write := func(db queryer) error {
		_, err := db.ExecContext(c.ctx, "INSERT INTO repositories(model_id,bundle,branch) VALUES($1,$2,$3) ON CONFLICT(model_id) DO UPDATE SET bundle=excluded.bundle,branch=excluded.branch", c.model, data, c.branch)
		if err == nil && !c.projectionOnly {
			err = versionstore.New(db).MoveGitHead(c.ctx, c.model, c.branch, head)
		}
		return err
	}
	if pool, ok := db.(*sql.DB); ok {
		tx, err := pool.BeginTx(c.ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err = write(tx); err != nil {
			return err
		}
		return tx.Commit()
	}
	return write(db)
}

type gitArgs struct {
	Model      string         `json:"model"`
	Script     string         `json:"script"`
	Note       string         `json:"note"`
	Source     string         `json:"source"`
	Inputs     map[string]any `json:"inputs"`
	Version    string         `json:"version"`
	Action     string         `json:"action"`
	Branch     string         `json:"branch"`
	Candidate  string         `json:"candidate"`
	AcceptedAt string         `json:"acceptedAt,omitempty"`
}

func gitState(c *candidate) (map[string]any, error) {
	refs, err := c.git("for-each-ref", "--format=%(refname:short)", "refs/heads")
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
	raw, err := c.git("log", "--branches", "--tags", "--topo-order", "-100", "--format=%H%x09%P%x09%s")
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
	result["commit"], err = c.git("rev-parse", "HEAD")
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
	if req.Op == "git.state" || req.Op == "git.prepare" {
		if err = historyProjectionReady(runtimeContext(req), db, a.Model); err != nil {
			return nil, err
		}
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
		return nil, importHistory(runtimeContext(req), db, req.Root, a.Model)
	}
	var snapshot RepositorySnapshot
	if req.Op == "git.prepare" && r.repositorySnapshot != nil {
		snapshot = *r.repositorySnapshot
		r.repositorySnapshot = nil
	} else {
		snapshot, err = repositorySnapshot(runtimeContext(req), db, a.Model)
	}
	if err != nil {
		return nil, err
	}
	c, err := workspaceSnapshot(runtimeContext(req), req.Root, a.Model, snapshot)
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
			_, err = c.git("checkout", "-b", a.Branch)
			c.branch = a.Branch
		case "checkout":
			_, err = c.git("checkout", a.Branch)
			c.branch = a.Branch
		case "merge":
			_, err = c.git("merge", "--no-ff", "--no-edit", a.Branch)
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
