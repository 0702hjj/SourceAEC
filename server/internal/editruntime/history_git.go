// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"ifcviewer/server/internal/versionstore"
)

// MaterializeSourceHistory writes at most one small batch per transaction.
// Canceling a large export preserves already committed batches for retry.
func (r *Runtime) MaterializeSourceHistory(ctx context.Context, root, token, model string) (bool, error) {
	db, err := r.historyDB(ctx, root, token, model)
	if err != nil {
		return false, err
	}
	if err = importHistory(ctx, db, root, model); err != nil {
		return false, err
	}
	store := versionstore.New(db)
	pending, err := store.Pending(ctx, model, 33)
	if err != nil {
		return false, err
	}
	if len(pending) == 0 {
		return true, nil
	}
	done := len(pending) <= 32
	pending = pending[:min(len(pending), 32)]
	snapshot, err := repositorySnapshot(ctx, db, model)
	if err != nil {
		return false, err
	}
	c, err := workspaceSnapshot(ctx, root, model, snapshot)
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(c.directory)
	c.projectionOnly = true
	for _, e := range pending {
		if c.branch != e.Branch {
			return false, errors.New("history branch differs from Git projection")
		}
		script, err := r.ReadStoredArtifact(ctx, root, token, e.ScriptKey)
		if err != nil {
			return false, err
		}
		inputs := map[string]any{}
		for name, key := range map[string]string{"current.inputs.json": e.InputsKey, "current.build.json": e.BuildKey, "current.map.json": e.MapKey} {
			data, err := r.ReadStoredArtifact(ctx, root, token, key)
			if err != nil {
				return false, err
			}
			value, err := archiveJSONValue(data)
			if err != nil {
				return false, err
			}
			inputs[name] = value
		}
		state, err := commitCandidate(c, gitArgs{Model: model, Script: string(script), Inputs: inputs, Note: e.Note, Source: e.Source, Version: e.Version, AcceptedAt: e.AcceptedAt.Format(time.RFC3339Nano)})
		if err != nil {
			return false, err
		}
		if err = store.MarkGit(ctx, model, e.Revision, state["commit"]); err != nil {
			return false, err
		}
	}
	return done, persist(db, c)
}

func archiveJSONValue(data []byte) (any, error) {
	if !json.Valid(data) {
		return nil, errors.New("invalid archived JSON")
	}
	var value any
	err := decode(data, &value)
	return value, err
}

func historyProjectionReady(ctx context.Context, db queryer, model string) error {
	pending, err := versionstore.New(db).Pending(ctx, model, 1)
	if err == nil && len(pending) > 0 {
		return versionstore.ErrProjectionPending
	}
	return err
}

func (r *Runtime) GitStateSnapshot(ctx context.Context, root, model string, snapshot RepositorySnapshot) (map[string]any, error) {
	c, err := workspaceSnapshot(ctx, root, model, snapshot)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(c.directory)
	return gitState(c)
}
