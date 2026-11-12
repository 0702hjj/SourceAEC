// SPDX-License-Identifier: Apache-2.0
package notebookcache_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/notebookcache"
)

func cacheFixture(t *testing.T, text string) notebookcache.Snapshot {
	t.Helper()
	root := t.TempDir()
	prefix := "skill-work/__marimo__"
	if err := os.MkdirAll(filepath.Join(root, prefix), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, prefix, "opaque.pkl"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := notebookcache.Pack(context.Background(), root, []string{prefix}, notebookcache.Hash([]byte("frozen environment")))
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func TestGenerationPublicationRollbackScopeGCAndRestart(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("dedicated PostgreSQL required")
	}
	root := t.TempDir()
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(root)
	db, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	model := "m_0123456789abcdef"
	initial := cacheFixture(t, "initial SDK cache")
	if err := notebookcache.New(db).Publish(ctx, model, initial); err == nil {
		t.Fatal("publication without model transaction accepted")
	}
	publish := func(value notebookcache.Snapshot, commit bool) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = notebookcache.New(tx).Publish(ctx, model, value); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	publish(initial, true)
	otherRoot := t.TempDir()
	if err = artifact.Open(otherRoot); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(otherRoot)
	otherDB, err := artifact.Database(otherRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = notebookcache.New(otherDB).Load(ctx, model); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("cross-namespace cache visible", err)
	}

	next := cacheFixture(t, "new SDK cache")
	publish(next, false)
	value, err := notebookcache.New(db).Load(ctx, model)
	if err != nil || value.Generation != initial.Generation {
		t.Fatal("rollback lost old generation", value, err)
	}
	if _, err = notebookcache.New(db).Load(ctx, "m_aaaaaaaaaaaaaaaa"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("cross-model cache visible", err)
	}
	// Regeneration repairs corrupt bytes at their original content identity.
	if _, err = db.Exec(`UPDATE notebook_generations SET payload='corrupt'::bytea WHERE model_id=$1`, model); err != nil {
		t.Fatal(err)
	}
	if _, err = notebookcache.New(db).Load(ctx, model); err == nil {
		t.Fatal("corrupt cache returned")
	}
	publish(initial, true)
	value, err = notebookcache.New(db).Load(ctx, model)
	if err != nil || value.Generation != initial.Generation {
		t.Fatal("regenerated cache did not repair damaged storage", err)
	}
	publish(next, true)
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM notebook_generations WHERE model_id=$1`, model).Scan(&count); err != nil || count != 1 {
		t.Fatal("obsolete generation not collected", count, err)
	}
	if err = artifact.Close(root); err != nil {
		t.Fatal(err)
	}
	if err = artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	db, err = artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	value, err = notebookcache.New(db).Load(ctx, model)
	if err != nil || value.Generation != next.Generation {
		t.Fatal("restart lost cache", value, err)
	}
	if err = notebookcache.New(db).DeleteModel(ctx, model); err != nil {
		t.Fatal(err)
	}
	if _, err = notebookcache.New(db).Load(ctx, model); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("deleted cache remains", err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM notebook_generations WHERE model_id=$1`, model).Scan(&count); err != nil || count != 0 {
		t.Fatal("deleted generation remains", count, err)
	}
}
