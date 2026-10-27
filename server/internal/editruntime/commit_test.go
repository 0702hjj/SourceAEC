// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"ifcviewer/server/internal/artifact"
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

func TestCommitExpiresOnlyOldReceipts(t *testing.T) {
	r, root := databaseRuntime(t)
	db, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO transaction_receipts(token,created_at) VALUES('old',clock_timestamp()-interval '11 minutes'),('recent',clock_timestamp())"); err != nil {
		t.Fatal(err)
	}
	tx := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "commit", root, tx, nil)
	var old, recent int
	if err = db.QueryRow("SELECT count(*) FILTER (WHERE token='old'),count(*) FILTER (WHERE token IN ('recent',$1)) FROM transaction_receipts", tx).Scan(&old, &recent); err != nil {
		t.Fatal(err)
	}
	if old != 0 || recent != 2 {
		t.Fatalf("receipt retention old=%d recent=%d", old, recent)
	}
}
