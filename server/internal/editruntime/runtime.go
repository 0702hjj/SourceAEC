// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
// Package editruntime owns storage and version-control operations. The gateway
// calls it in-process; the standalone command also exposes private JSON stdio.
package editruntime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ifcviewer/server/internal/artifact"
)

type Request struct {
	Context context.Context `json:"-"`
	Op      string          `json:"op"`
	Root    string          `json:"root"`
	Tx      string          `json:"tx,omitempty"`
	Args    json.RawMessage `json:"args"`
}
type Response struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}
type queryer interface {
	Exec(string, ...any) (sql.Result, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type transaction struct {
	tx            *sql.Tx
	root          string
	tracked       map[string]struct{}
	token, model  string
	journalLocked bool
	readOnly      bool
}
type candidate struct {
	directory, repo, branch, root, model string
	ctx                                  context.Context
	projectionOnly                       bool
}
type Runtime struct {
	embedded           bool
	opened             map[string]bool
	transactions       map[string]*transaction
	candidates         map[string]*candidate
	commitTx           func(*sql.Tx) error
	repositorySnapshot *RepositorySnapshot
}

func New() *Runtime {
	return &Runtime{opened: map[string]bool{}, transactions: map[string]*transaction{}, candidates: map[string]*candidate{}}
}

// NewEmbedded shares the gateway database lifecycle.
func NewEmbedded() *Runtime { r := New(); r.embedded = true; return r }

func token() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func decode(raw json.RawMessage, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(value)
}
func (r *Runtime) database(req Request) (queryer, error) {
	if req.Tx != "" {
		t := r.transactions[req.Tx]
		if t == nil || t.root != req.Root {
			return nil, errors.New("invalid transaction")
		}
		return t.tx, nil
	}
	r.opened[req.Root] = true
	return artifact.Database(req.Root)
}
func (r *Runtime) Close() {
	for token, t := range r.transactions {
		_ = t.tx.Rollback()
		if t.journalLocked && restore(t) == nil {
			artifact.ForgetProjections(t.root, token)
		}
	}
	for _, c := range r.candidates {
		_ = os.RemoveAll(c.directory)
	}
	for root := range r.opened {
		if r.embedded {
			continue
		}
		_ = artifact.Close(root)
	}
}
func (r *Runtime) Handle(req Request) (any, error) {
	if req.Op == "ping" {
		return map[string]string{"runtime": "go", "storage": "postgresql"}, nil
	}
	if req.Op == "durable" {
		var path string
		if err := decode(req.Args, &path); err != nil {
			return nil, err
		}
		return artifact.Durable(path), nil
	}
	if req.Op == "release" {
		for _, t := range r.transactions {
			if t.root == req.Root {
				return nil, errors.New("workspace has an active transaction")
			}
		}
		for _, c := range r.candidates {
			if c.root == req.Root {
				return nil, errors.New("workspace has an active Git candidate")
			}
		}
		delete(r.opened, req.Root)
		return nil, artifact.Close(req.Root)
	}
	if req.Op == "initialize" {
		if r.opened[req.Root] {
			return nil, r.restoreWorkspace(req.Root)
		}
		if err := artifact.Open(req.Root); err != nil {
			return nil, err
		}
		r.opened[req.Root] = true
		return nil, nil
	}
	if req.Op == "begin" || req.Op == "begin_read" {
		r.opened[req.Root] = true
		db, err := artifact.Database(req.Root)
		if err != nil {
			return nil, err
		}
		var options *sql.TxOptions
		if req.Op == "begin_read" {
			options = &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
		}
		tx, err := db.BeginTx(runtimeContext(req), options)
		if err != nil {
			return nil, err
		}
		id := token()
		r.transactions[id] = &transaction{tx: tx, root: req.Root, token: id, tracked: map[string]struct{}{}}
		r.transactions[id].readOnly = options != nil
		return id, nil
	}
	if req.Op == "lock" {
		t := r.transactions[req.Tx]
		if t == nil || t.root != req.Root {
			return nil, errors.New("invalid transaction")
		}
		var model string
		if err := decode(req.Args, &model); err != nil {
			return nil, err
		}
		if err := artifact.LockModel(runtimeContext(req), t.tx, req.Root, model); err != nil {
			return nil, err
		}
		t.model = model
		return nil, nil
	}
	if req.Op == "commit" || req.Op == "rollback" {
		t := r.transactions[req.Tx]
		if t == nil || t.root != req.Root {
			return nil, errors.New("invalid transaction")
		}
		delete(r.transactions, req.Tx)
		if req.Op == "rollback" {
			err := t.tx.Rollback()
			e := restore(t)
			if t.journalLocked && e == nil {
				artifact.ForgetProjections(t.root, req.Tx)
			}
			if err != nil {
				return nil, err
			}
			return nil, e
		}
		if err := r.commit(t); err != nil {
			if t.journalLocked && restore(t) == nil {
				artifact.ForgetProjections(t.root, req.Tx)
			}
			return nil, err
		}
		if t.journalLocked {
			artifact.ForgetProjections(t.root, req.Tx)
		}
		return nil, nil
	}
	if req.Op == "track" {
		var path string
		if err := decode(req.Args, &path); err != nil {
			return nil, err
		}
		return nil, r.track(req, path)
	}
	if len(req.Op) > 5 && req.Op[:5] == "edit." {
		return bindingOperation(req)
	}
	if len(req.Op) > 4 && req.Op[:4] == "git." {
		return r.gitOperation(req)
	}
	if len(req.Op) > 6 && req.Op[:6] == "build." {
		return r.buildOperation(req)
	}
	return r.artifactOperation(req)
}
func Serve(input io.Reader, output io.Writer) error {
	runtime := New()
	defer runtime.Close()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), 512<<20)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var request Request
		var response Response
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			response.Error = "invalid runtime request"
		} else {
			value, err := runtime.Handle(request)
			if err != nil {
				response.Error = err.Error()
			} else {
				response.Data = value
			}
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}
func safePath(root, key string) (string, error) {
	if key == "" || filepath.IsAbs(key) {
		return "", errors.New("invalid artifact path")
	}
	clean := filepath.Clean(key)
	if clean == ".." || len(clean) > 3 && clean[:3] == "../" {
		return "", errors.New("invalid artifact path")
	}
	path := filepath.Join(root, clean)
	// Reject symlink components, including a dangling final symlink.
	for current := path; current != root; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symlink artifact path")
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if filepath.Dir(current) == current {
			return "", fmt.Errorf("path escapes root")
		}
	}
	return path, nil
}

func runtimeContext(req Request) context.Context {
	if req.Context != nil {
		return req.Context
	}
	return context.Background()
}
