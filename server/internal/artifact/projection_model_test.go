// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestModelRecoveryDoesNotTouchOtherLiveWriter(t *testing.T) {
	root := journalStore(t)
	first, second := "m_0000000000000001", "m_0000000000000002"
	a, b := "uploads/"+first+".ifc", "uploads/"+second+".ifc"
	for _, key := range []string{a, b} {
		if err := WriteFile(filepath.Join(root, key), []byte("committed"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	db, err := Database(root)
	if err != nil {
		t.Fatal(err)
	}
	live, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer live.Rollback()
	if err = LockModel(context.Background(), live, root, second); err != nil {
		t.Fatal(err)
	}
	if err = LockProjectionRecovery(context.Background(), live, root, true); err != nil {
		t.Fatal(err)
	}
	for token, key := range map[string]string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": a, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": b} {
		if err = RecordProjection(root, token, key); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, key), []byte("uncommitted"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	repair, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer repair.Rollback()
	if err = LockModel(context.Background(), repair, root, first); err != nil {
		t.Fatal(err)
	}
	quarantined, err := RecoverModelProjections(context.Background(), root, repair, first, "cccccccccccccccccccccccccccccccc")
	if err != nil {
		t.Fatal(err)
	}
	if !quarantined[a] || quarantined[b] {
		t.Fatal("recovery did not isolate interrupted paths", quarantined)
	}
	if err = repair.Commit(); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{a: "committed", b: "uncommitted"} {
		data, err := os.ReadFile(filepath.Join(root, key))
		if err != nil || string(data) != want {
			t.Fatal("model-scoped recovery crossed writer boundaries", key, string(data), err)
		}
	}
	if err = WriteFile(filepath.Join(root, a), []byte("new committed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = RecoverProjectionTransaction(root, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", first); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, a))
	if err != nil || string(data) != "new committed" {
		t.Fatal("old rollback overwrote a newer commit", string(data), err)
	}
}
