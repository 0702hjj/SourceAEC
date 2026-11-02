// SPDX-License-Identifier: Apache-2.0
package workflow_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/workflow"
)

func TestWorkflowCheckpointSurvivesPoolReopen(t *testing.T) {
	db, root := database(t)
	ctx := context.Background()
	created := create(t, db, creation("reopen"))
	owned := claim(t, db, "original")
	if err := transaction(t, db, func(s *workflow.Store) error { return s.CompleteGeometry(ctx, owned.Fence(), reference(model)) }); err != nil {
		t.Fatal(err)
	}
	if err := artifact.Close(root); err != nil {
		t.Fatal(err)
	}
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	reopened, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	next := claim(t, reopened, "restarted")
	if next.ID != created.ID || next.Phase != workflow.Publication || next.Candidate == nil {
		t.Fatal("checkpoint lost on restart", next)
	}
}

func TestWorkflowDXFPublicationFinishesWithoutConversion(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	in := creation("dxf")
	in.Definition.Kind = "dxf"
	created := create(t, db, in)
	geometry := claim(t, db, "geometry")
	if err := transaction(t, db, func(s *workflow.Store) error { return s.CompleteGeometry(ctx, geometry.Fence(), reference(model)) }); err != nil {
		t.Fatal(err)
	}
	pub := claim(t, db, "publication")
	for _, value := range []string{"null", "[]", "1", "{"} {
		if err := transaction(t, db, func(s *workflow.Store) error { return s.CompletePublication(ctx, pub.Fence(), []byte(value)) }); err == nil {
			t.Fatal("invalid publication result accepted", value)
		}
	}
	if err := transaction(t, db, func(s *workflow.Store) error {
		return s.CompletePublication(ctx, pub.Fence(), []byte(`{"version":"v1"}`))
	}); err != nil {
		t.Fatal(err)
	}
	run, err := workflow.New(db).Get(ctx, model, created.ID)
	if err != nil || run.State != workflow.Succeeded || run.Phase != workflow.Publication {
		t.Fatal(run, err)
	}
}

func expire(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec("UPDATE workflow_runs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowLeaseRecoveryFencesOldWorker(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	created := create(t, db, creation("lease"))
	first := claim(t, db, "original")
	expire(t, db, created.ID)
	if err := workflow.New(db).Heartbeat(ctx, first.Fence(), time.Minute); !errors.Is(err, workflow.ErrLeaseLost) {
		t.Fatal("expired owner revived its lease", err)
	}
	second := claim(t, db, "replacement")
	if second.Token == first.Token || second.Attempts != 2 {
		t.Fatal("attempt reused", second)
	}
	if err := workflow.New(db).Heartbeat(ctx, first.Fence(), time.Minute); !errors.Is(err, workflow.ErrLeaseLost) {
		t.Fatal("old heartbeat accepted", err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error { return s.CompleteGeometry(ctx, first.Fence(), reference(model)) }); !errors.Is(err, workflow.ErrLeaseLost) {
		t.Fatal("old result accepted", err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error { return s.CompleteGeometry(ctx, second.Fence(), reference(model)) }); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow("SELECT state FROM workflow_attempts WHERE run_id=$1 AND attempt=1", created.ID).Scan(&state); err != nil || state != "abandoned" {
		t.Fatal(state, err)
	}
}

func TestWorkflowCompetingClaimsSkipLockedRun(t *testing.T) {
	db, _ := database(t)
	create(t, db, creation("first"))
	create(t, db, creation("second"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	firstTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer firstTx.Rollback()
	first, firstFound, err := workflow.New(firstTx).Claim(ctx, "first-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	secondTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer secondTx.Rollback()
	second, secondFound, err := workflow.New(secondTx).Claim(ctx, "second-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !firstFound || !secondFound || first.ID == second.ID {
		t.Fatal("two workers own same run")
	}
	if err := secondTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := firstTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error {
		_, found, err := s.Claim(ctx, "third-worker", time.Minute)
		if found {
			t.Fatal("live work reclaimed")
		}
		return err
	}); err != nil {
		t.Fatal("claim failed", err)
	}
}

func TestWorkflowRetryLimitAndExplicitResume(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	created := create(t, db, creation("attempts"))
	for i := 1; i <= workflow.MaxPhaseAttempts; i++ {
		r := claim(t, db, "worker")
		if r.Attempts != i {
			t.Fatal(r.Attempts)
		}
		expire(t, db, r.ID)
	}
	// Commit the exhaustion sweep even when no runnable work remains.
	if err := transaction(t, db, func(s *workflow.Store) error {
		_, found, err := s.Claim(ctx, "worker", time.Minute)
		if found {
			t.Fatal("exhausted run reclaimed")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r, err := workflow.New(db).Get(ctx, model, created.ID)
	if err != nil || r.State != workflow.Failed || r.Attempts != 3 {
		t.Fatal(r, err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error {
		resumed, err := s.Resume(ctx, model, r.ID)
		if err == nil && resumed.Definition != r.Definition {
			t.Fatal("resume changed definition")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	fourth := claim(t, db, "explicit-resume")
	if fourth.Attempts != 4 || fourth.PhaseAttempts != 1 {
		t.Fatal(fourth)
	}
	message := strings.Repeat("源", 1000) + "\x00"
	if err := transaction(t, db, func(s *workflow.Store) error { return s.Fail(ctx, fourth.Fence(), message, false) }); err != nil {
		t.Fatal(err)
	}
	r, err = workflow.New(db).Get(ctx, model, r.ID)
	if err != nil || r.State != workflow.Failed || len(r.Error) > 2048 || !utf8.ValidString(r.Error) || strings.ContainsRune(r.Error, 0) {
		t.Fatal("invalid persisted error", r, err)
	}
}

func TestWorkflowCancelAndResumeKeepCheckpoint(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	created := create(t, db, creation("cancel"))
	geometry := claim(t, db, "geometry")
	if err := transaction(t, db, func(s *workflow.Store) error { return s.CompleteGeometry(ctx, geometry.Fence(), reference(model)) }); err != nil {
		t.Fatal(err)
	}
	publication := claim(t, db, "publication")
	if err := transaction(t, db, func(s *workflow.Store) error { _, err := s.Cancel(ctx, model, created.ID); return err }); err != nil {
		t.Fatal(err)
	}
	if err := workflow.New(db).Heartbeat(ctx, publication.Fence(), time.Minute); !errors.Is(err, workflow.ErrLeaseLost) {
		t.Fatal("cancel did not fence publisher", err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error {
		r, err := s.Resume(ctx, model, created.ID)
		if err == nil && (r.Candidate == nil || r.Phase != workflow.Publication || r.Definition != created.Definition) {
			t.Fatal("resume lost checkpoint", r)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r := claim(t, db, "resumed")
	if r.Phase != workflow.Publication || r.PhaseAttempts != 1 {
		t.Fatal("resume reran geometry", r)
	}
}

func TestWorkflowRetryBackoff(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	create(t, db, creation("retry"))
	r := claim(t, db, "worker")
	if err := transaction(t, db, func(s *workflow.Store) error { return s.Fail(ctx, r.Fence(), "transient", true) }); err != nil {
		t.Fatal(err)
	}
	var delay float64
	if err := db.QueryRow("SELECT extract(epoch FROM (available_at-updated_at)) FROM workflow_runs WHERE id=$1", r.ID).Scan(&delay); err != nil || delay != 2 {
		t.Fatal("incorrect persisted backoff", delay, err)
	}
	// Move the clock boundary explicitly; runner scheduling must not decide
	// whether the backoff has elapsed on a loaded shared CI machine.
	if _, err := db.Exec("UPDATE workflow_runs SET available_at=clock_timestamp()+interval '1 hour' WHERE id=$1", r.ID); err != nil {
		t.Fatal(err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error {
		_, found, err := s.Claim(ctx, "worker", time.Minute)
		if found {
			t.Fatal("retry ignored backoff")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE workflow_runs SET available_at=clock_timestamp()-interval '1 second' WHERE id=$1", r.ID); err != nil {
		t.Fatal(err)
	}
	if next := claim(t, db, "worker"); next.Attempts != 2 {
		t.Fatal(next)
	}
}
