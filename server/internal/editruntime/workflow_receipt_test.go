// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/versionstore"
	"ifcviewer/server/internal/workflow"
)

func TestWorkflowPublicationLostAcknowledgement(t *testing.T) {
	for _, committed := range []bool{true, false} {
		t.Run(map[bool]string{true: "committed", false: "aborted"}[committed], func(t *testing.T) {
			r, root := databaseRuntime(t)
			ctx := context.Background()
			model := "m_0000000000000001"
			db, err := artifact.Database(root)
			if err != nil {
				t.Fatal(err)
			}
			step := func(fn func(*workflow.Store) error) {
				t.Helper()
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if err = fn(workflow.New(tx)); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			hash := strings.Repeat("a", 64)
			ref := workflow.Ref{Key: "models/" + model + "/workflows/blobs/" + hash, SHA256: hash, Bytes: 1}
			var run workflow.Run
			step(func(s *workflow.Store) error {
				var e error
				run, _, e = s.Create(ctx, workflow.Create{Model: model, Key: "receipt", BaseIdentity: hash, Intent: workflow.Intent{Action: "save", BaseRevision: hash, Source: "ai"}, Definition: workflow.Definition{Schema: 1, Model: model, Kind: "dxf", Engine: "fixture", EnvironmentDigest: hash, Script: ref, Inputs: ref, Environment: ref}})
				return e
			})
			step(func(s *workflow.Store) error {
				var e error
				run, _, e = s.Claim(ctx, "geometry", time.Minute)
				if e != nil {
					return e
				}
				return s.CompleteGeometry(ctx, run.Fence(), ref)
			})
			step(func(s *workflow.Store) error {
				var e error
				run, _, e = s.Claim(ctx, "publisher", time.Minute)
				return e
			})
			token := invoke(t, r, "begin", root, "", nil).(string)
			invoke(t, r, "lock", root, token, model)
			if err = r.EnsureSourceHistory(ctx, root, token, model); err != nil {
				t.Fatal(err)
			}
			if _, err = r.NextVersionNumber(ctx, root, token, model); err != nil {
				t.Fatal(err)
			}
			prefix := "models/" + model + "/scripts/v1"
			invoke(t, r, "publish", root, token, []fileEntry{{prefix + ".py", []byte("PARAMS = {}")}, {prefix + ".inputs.json", []byte("{}")}, {prefix + ".build.json", []byte("{}")}, {prefix + ".map.json", []byte("{}")}})
			revision, _ := versionstore.NewRevision()
			if _, err = r.AppendSourceHistory(ctx, root, token, model, versionstore.Event{Revision: revision, Version: "v1", Source: "ai", ScriptKey: prefix + ".py", InputsKey: prefix + ".inputs.json", BuildKey: prefix + ".build.json", MapKey: prefix + ".map.json"}); err != nil {
				t.Fatal(err)
			}
			if err = r.WorkflowModel(ctx, root, token, model, func(s *workflow.Store) error {
				return s.CompletePublication(ctx, run.Fence(), json.RawMessage(`{"response":{"version":"v1"}}`))
			}); err != nil {
				t.Fatal(err)
			}
			r.commitTx = func(tx *sql.Tx) error {
				if committed {
					if e := tx.Commit(); e != nil {
						return e
					}
				} else {
					_ = tx.Rollback()
				}
				return io.EOF
			}
			_, err = r.Handle(Request{Op: "commit", Root: root, Tx: token})
			if (err == nil) != committed {
				t.Fatal(committed, err)
			}
			got, err := workflow.New(db).Get(ctx, model, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			var count int
			if err = db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", model).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if committed {
				if count != 1 || got.State != workflow.Succeeded {
					t.Fatal(count, got)
				}
			} else {
				if count != 0 || got.Phase != workflow.Publication || got.Result != nil {
					t.Fatal(count, got)
				}
			}
		})
	}
}
