// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"io"
	"testing"
)

func TestCommitReconcilesLostAcknowledgement(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{true: "committed", false: "aborted"}[committed], func(t *testing.T) {
			r, root := databaseRuntime(t)
			tx := invoke(t, r, "begin", root, "", nil).(string)
			invoke(t, r, "lock", root, tx, "m_0000000000000001")
			key := "models/m_0000000000000001/scripts/v1.py"
			invoke(t, r, "publish", root, tx, []fileEntry{{key, []byte("saved")}})
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
			_, err := r.Handle(Request{Root: root, Tx: tx, Op: "commit"})
			if (err == nil) != committed {
				t.Fatalf("committed=%v error=%v", committed, err)
			}
			data, err := r.ReadStoredArtifact(context.Background(), root, "", key)
			if committed && (err != nil || string(data) != "saved") {
				t.Fatalf("lost committed state: %s %v", data, err)
			}
			if !committed && err == nil {
				t.Fatal("aborted state was visible")
			}
		})
	}
}
