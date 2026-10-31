// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"ifcviewer/server/internal/versionstore"
)

func (r *Runtime) historyDB(ctx context.Context, root, token, model string) (queryer, error) {
	t := r.transactions[token]
	if t == nil || t.root != root || t.readOnly || t.model != model {
		return nil, errors.New("history write requires a model transaction")
	}
	return r.database(Request{Context: ctx, Root: root, Tx: token})
}

func (r *Runtime) SourceHistoryReady(ctx context.Context, root, model string) (bool, error) {
	db, err := r.database(Request{Context: ctx, Root: root})
	if err != nil {
		return false, err
	}
	_, err = versionstore.New(db).Head(ctx, model)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// EnsureSourceHistory imports only the old head/number once. Existing bundles
// remain untouched and preserve their branch DAG. New models need no Git tool.
func (r *Runtime) EnsureSourceHistory(ctx context.Context, root, token, model string) error {
	if !versionstore.ValidModel(model) {
		return errors.New("invalid source history model")
	}
	db, err := r.historyDB(ctx, root, token, model)
	if err != nil {
		return err
	}
	store := versionstore.New(db)
	if _, err = store.Head(ctx, model); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	h := versionstore.Head{Branch: "main"}
	// Initialize the counter without loading every historical metadata document.
	err = db.QueryRowContext(ctx, `SELECT coalesce(max((substring(path FROM '/v([1-9][0-9]*)[.]'))::bigint),0)
 FROM artifacts WHERE path ~ $1`, "^models/"+model+"/scripts/v[1-9][0-9]*[.](py|product)$").Scan(&h.LastNumber)
	if err != nil {
		return err
	}
	if h.LastNumber > 0 {
		h.Ref = fmt.Sprintf("legacy:v%d", h.LastNumber)
	}
	snapshot, err := repositorySnapshot(ctx, db, model)
	if err != nil {
		return err
	}
	if len(snapshot.Bundle) > 0 {
		c, err := workspaceSnapshot(ctx, root, model, snapshot)
		if err != nil {
			return err
		}
		defer os.RemoveAll(c.directory)
		head, err := c.git("rev-parse", "HEAD")
		if err != nil {
			return err
		}
		h.Ref, h.Branch = "git:"+head, c.branch
		tags, err := c.git("tag", "--list", "v[0-9]*")
		if err != nil {
			return err
		}
		for _, tag := range strings.Fields(tags) {
			if n, err := strconv.ParseInt(strings.TrimPrefix(tag, "v"), 10, 64); err == nil {
				h.LastNumber = max(h.LastNumber, n)
			}
		}
	}
	return store.Initialize(ctx, model, h)
}

func (r *Runtime) NextVersionNumber(ctx context.Context, root, token, model string) (int64, error) {
	db, err := r.historyDB(ctx, root, token, model)
	if err != nil {
		return 0, err
	}
	return versionstore.New(db).NextNumber(ctx, model)
}

func (r *Runtime) AppendSourceHistory(ctx context.Context, root, token, model string, e versionstore.Event) (versionstore.Event, error) {
	db, err := r.historyDB(ctx, root, token, model)
	if err != nil {
		return versionstore.Event{}, err
	}
	for _, key := range []string{e.ScriptKey, e.InputsKey, e.BuildKey, e.MapKey} {
		if err = r.RequireStoredArtifact(ctx, root, token, key); err != nil {
			return versionstore.Event{}, err
		}
	}
	return versionstore.New(db).Append(ctx, model, e)
}
