// SPDX-License-Identifier: Apache-2.0
package workflow_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/workflow"
)

const model = "m_0123456789abcdef"

func database(t *testing.T) (*sql.DB, string) {
	t.Helper()
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("dedicated PostgreSQL required")
	}
	t.Setenv("VIEWER_STORAGE_NAMESPACE", "")
	root := t.TempDir()
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifact.Close(root) })
	db, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Schema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db, root
}

func transaction(t *testing.T, db *sql.DB, fn func(*workflow.Store) error) error {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = fn(workflow.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func reference(m string) workflow.Ref {
	hash := strings.Repeat("a", 64)
	return workflow.Ref{Key: "models/" + m + "/workflows/blobs/" + hash, SHA256: hash, Bytes: 1}
}

func creation(key string) workflow.Create {
	ref := reference(model)
	return workflow.Create{Model: model, Key: key, BaseIdentity: strings.Repeat("b", 64),
		Definition: workflow.Definition{Schema: 1, Model: model, Kind: "ifc", Engine: "fixture.v1", EnvironmentDigest: strings.Repeat("c", 64), Script: ref, Inputs: ref, Environment: ref},
		Intent:     workflow.Intent{Action: "save", BaseRevision: strings.Repeat("d", 64), Source: "ai", Note: "synthetic"}}
}

func create(t *testing.T, db *sql.DB, in workflow.Create) workflow.Run {
	t.Helper()
	var run workflow.Run
	err := transaction(t, db, func(s *workflow.Store) error {
		var err error
		run, _, err = s.Create(context.Background(), in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func claim(t *testing.T, db *sql.DB, owner string) workflow.Run {
	t.Helper()
	var run workflow.Run
	err := transaction(t, db, func(s *workflow.Store) error {
		var err error
		var found bool
		run, found, err = s.Claim(context.Background(), owner, time.Minute)
		if err == nil && !found {
			t.Fatal("expected a runnable workflow")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestWorkflowDefinitionIdempotencyAndGuards(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	in := creation("request-1")
	in.Intent.Note = "synthetic\x00来源"
	run := create(t, db, in)
	got, err := workflow.New(db).Definition(ctx, model, run.Definition)
	if err != nil || !reflect.DeepEqual(got, in.Definition) {
		t.Fatal(got, err)
	}
	if _, _, err := workflow.New(db).Create(ctx, in); err == nil {
		t.Fatal("creation without transaction accepted")
	}
	in.Definition.Engine = "different.v2"
	in.BaseIdentity = strings.Repeat("e", 64)
	err = transaction(t, db, func(s *workflow.Store) error {
		duplicate, created, err := s.Create(ctx, in)
		if err == nil && (created || duplicate.ID != run.ID || duplicate.Definition != run.Definition || duplicate.Intent.Note != run.Intent.Note) {
			t.Fatal("retry redefined original run", duplicate)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	in.Intent.Note = "different"
	if err := transaction(t, db, func(s *workflow.Store) error { _, _, err := s.Create(ctx, in); return err }); !errors.Is(err, workflow.ErrConflict) {
		t.Fatal("same key changed intent", err)
	}
	if _, err := db.Exec("UPDATE workflow_definitions SET body=$1 WHERE digest=$2", []byte(`{}`), run.Definition); err == nil {
		t.Fatal("definition body changed without digest")
	}
	bad := creation("invalid")
	bad.Definition.Inputs = reference("m_0000000000000001")
	if _, _, err := bad.Definition.Encode(); err == nil {
		t.Fatal("cross-model reference accepted")
	}
	bad = creation("invalid")
	bad.Definition.Script.Key = "../escape"
	if _, _, err := bad.Definition.Encode(); err == nil {
		t.Fatal("mutable/escaping blob reference accepted")
	}
	if _, err := workflow.New(db).List(ctx, model, 101); err == nil {
		t.Fatal("unbounded list accepted")
	}
}

func TestWorkflowCheckpointAndPublicationAtomicity(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	created := create(t, db, creation("atomic"))
	geometry := claim(t, db, "worker-1")
	if err := transaction(t, db, func(s *workflow.Store) error { return s.CompleteGeometry(ctx, geometry.Fence(), reference(model)) }); err != nil {
		t.Fatal(err)
	}
	publication := claim(t, db, "worker-2")
	if publication.Phase != workflow.Publication || publication.PhaseAttempts != 1 || publication.Candidate == nil {
		t.Fatal(publication)
	}
	if _, err := db.Exec("CREATE TABLE synthetic_publications(run_id TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("INSERT INTO synthetic_publications VALUES($1)", created.ID); err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"version":"v1"}`)
	if err := workflow.New(tx).CompletePublication(ctx, publication.Fence(), result); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	run, err := workflow.New(db).Get(ctx, model, created.ID)
	if err != nil || run.Phase != workflow.Publication || run.Result != nil {
		t.Fatal("rolled-back publication visible", run, err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM synthetic_publications").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO synthetic_publications VALUES($1)", created.ID); err != nil {
		t.Fatal(err)
	}
	if err = workflow.New(tx).CompletePublication(ctx, publication.Fence(), result); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	conversion := claim(t, db, "worker-3")
	if conversion.Phase != workflow.Conversion || string(conversion.Result) != string(result) {
		t.Fatal("checkpoint not retained", conversion)
	}
	if err := transaction(t, db, func(s *workflow.Store) error { return s.CompleteConversion(ctx, conversion.Fence()) }); err != nil {
		t.Fatal(err)
	}
	if err := workflow.New(db).Heartbeat(ctx, publication.Fence(), time.Minute); !errors.Is(err, workflow.ErrLeaseLost) {
		t.Fatal("old publisher retained lease", err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error { _, err := s.Resume(ctx, model, created.ID); return err }); !errors.Is(err, workflow.ErrState) {
		t.Fatal("completed run resumed", err)
	}
	run, err = workflow.New(db).Get(ctx, model, created.ID)
	if err != nil || run.State != workflow.Succeeded || run.Attempts != 3 {
		t.Fatal(run, err)
	}
}
