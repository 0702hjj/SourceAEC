// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/workflow"
)

type inputWorkflowExecutor struct {
	workflowFixtureExecutor
	captured map[string]string
}

func (f *inputWorkflowExecutor) RunFrozen(ctx context.Context, script, kind string, seal EnvironmentSeal) (Product, error) {
	f.captured, _ = ctx.Value(inputContextKey{}).(map[string]string)
	return f.workflowFixtureExecutor.RunFrozen(ctx, script, kind, seal)
}
func TestWorkflowFrozenInputsAndPublicationConflict(t *testing.T) {
	w, id, _, request := workflowFixture(t, "dxf")
	ctx := context.Background()
	// Attach synthetic project inputs while preserving model incarnation fields.
	model, err := w.models.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	model.ProjectID = "p_workflow"
	if err = artifact.WriteFile(filepath.Join(w.root, "models", id, "model.json"), jsonBytes(model), 0600); err != nil {
		t.Fatal(err)
	}
	key := "plans/p_workflow/plan.json"
	value := []byte(`{"revision":"frozen"}`)
	if err = artifact.WriteFile(filepath.Join(w.root, key), value, 0600); err != nil {
		t.Fatal(err)
	}
	f := &inputWorkflowExecutor{}
	w.services["dxf"].Executor = f
	r, _, err := w.Submit(ctx, id, request)
	if err != nil {
		t.Fatal(err)
	}
	f.entered = make(chan struct{})
	f.gate = make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := w.runOne(ctx); done <- err }()
	select {
	case <-f.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("geometry not started")
	}
	if err = artifact.WriteFile(filepath.Join(w.root, key), []byte(`{"revision":"new"}`), 0600); err != nil {
		t.Fatal(err)
	}
	close(f.gate)
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("geometry did not finish")
	}
	raw, err := base64.StdEncoding.DecodeString(f.captured[key])
	if err != nil || string(raw) != string(value) {
		t.Fatal("running workflow used live inputs", string(raw), err)
	}
	workflowStep(t, w)
	v, err := w.Get(ctx, id, r.ID)
	if err != nil || v.State != workflow.Superseded {
		t.Fatal(v, err)
	}
	var count int
	if err = w.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&count); err != nil || count != 0 {
		t.Fatal("stale input publication", count, err)
	}
}
func TestWorkflowReclaimedPublisherCannotWriteVersion(t *testing.T) {
	w, id, _, request := workflowFixture(t, "dxf")
	ctx := context.Background()
	r, _, err := w.Submit(ctx, id, request)
	if err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	claim := func(owner string) workflow.Run {
		t.Helper()
		var run workflow.Run
		if err := w.transaction(ctx, func(s *workflow.Store) error {
			var err error
			run, _, err = s.Claim(ctx, owner, time.Minute)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return run
	}
	old := claim("old-publisher")
	if _, err = w.db.Exec("UPDATE workflow_runs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", r.ID); err != nil {
		t.Fatal(err)
	}
	current := claim("new-publisher")
	if err = w.publication(ctx, old); !errors.Is(err, workflow.ErrLeaseLost) {
		t.Fatal("late owner accepted", err)
	}
	var count int
	if err = w.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = w.publication(ctx, current); err != nil {
		t.Fatal(err)
	}
	got, err := w.Get(ctx, id, r.ID)
	if err != nil || got.State != workflow.Succeeded {
		data, _ := json.Marshal(got)
		t.Fatal(string(data), err)
	}
}
