// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"io"
	"testing"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/versionstore"
)

func TestSourceHistoryCommitReceiptKeepsRecordAndNumberAtomic(t *testing.T) {
	for _, committed := range []bool{true, false} {
		t.Run(map[bool]string{true: "committed", false: "aborted"}[committed], func(t *testing.T) {
			r, root := databaseRuntime(t)
			ctx := context.Background()
			model := "m_0000000000000001"
			tx := invoke(t, r, "begin", root, "", nil).(string)
			invoke(t, r, "lock", root, tx, model)
			if err := r.EnsureSourceHistory(ctx, root, tx, model); err != nil {
				t.Fatal(err)
			}
			if _, err := r.NextVersionNumber(ctx, root, tx, model); err != nil {
				t.Fatal(err)
			}
			prefix := "models/" + model + "/scripts/v1"
			invoke(t, r, "publish", root, tx, []fileEntry{{prefix + ".py", []byte("PARAMS = {}")}, {prefix + ".inputs.json", []byte("{}")}, {prefix + ".build.json", []byte("{}")}, {prefix + ".map.json", []byte("{}")}})
			id, _ := versionstore.NewRevision()
			if _, err := r.AppendSourceHistory(ctx, root, tx, model, versionstore.Event{Revision: id, Version: "v1", Source: "human", ScriptKey: prefix + ".py", InputsKey: prefix + ".inputs.json", BuildKey: prefix + ".build.json", MapKey: prefix + ".map.json"}); err != nil {
				t.Fatal(err)
			}
			r.commitTx = func(tx *sql.Tx) error {
				if committed {
					if err := tx.Commit(); err != nil {
						return err
					}
				} else {
					_ = tx.Rollback()
				}
				return io.EOF
			}
			_, err := r.Handle(Request{Op: "commit", Root: root, Tx: tx})
			if (err == nil) != committed {
				t.Fatal(committed, err)
			}
			db, _ := artifact.Database(root)
			var records, heads int
			if err := db.QueryRow("SELECT (SELECT count(*) FROM version_events),(SELECT count(*) FROM version_heads)").Scan(&records, &heads); err != nil {
				t.Fatal(err)
			}
			want := 0
			if committed {
				want = 1
			}
			if records != want || heads != want {
				t.Fatal("receipt outcome disagrees with history state", records, heads)
			}
		})
	}
}

func TestSourceHistoryRequiresOwningModelTransaction(t *testing.T) {
	r, root := databaseRuntime(t)
	tx := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "lock", root, tx, "m_0000000000000001")
	if err := r.EnsureSourceHistory(context.Background(), root, tx, "m_0000000000000002"); err == nil {
		t.Fatal("cross-model history write accepted")
	}
}
