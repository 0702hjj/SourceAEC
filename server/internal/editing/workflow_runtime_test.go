// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
	"ifcviewer/server/internal/workflow"
)

type workflowFixtureExecutor struct {
	calls   atomic.Int32
	drift   atomic.Bool
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func (f *workflowFixtureExecutor) Freeze(context.Context, string, string) (EnvironmentSeal, error) {
	label := "fixture"
	if f.drift.Load() {
		label = "changed"
	}
	return EnvironmentSeal{1, "fixture", digest([]byte(label))}, nil
}
func (f *workflowFixtureExecutor) Run(ctx context.Context, script, kind string) (Product, error) {
	return (&testExecutor{}).Run(ctx, script, kind)
}
func (f *workflowFixtureExecutor) RunFrozen(ctx context.Context, script, kind string, seal EnvironmentSeal) (Product, error) {
	actual, _ := f.Freeze(ctx, script, kind)
	if actual != seal {
		return Product{}, failure(503, "environment changed")
	}
	f.calls.Add(1)
	if f.entered != nil {
		f.once.Do(func() { close(f.entered) })
	}
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return Product{}, ctx.Err()
		}
	}
	p, err := f.Run(ctx, script, kind)
	p.Environment = seal.Digest
	return p, err
}
func workflowFixture(t *testing.T, kind string) (*Workflows, string, *workflowFixtureExecutor, WorkflowRequest) {
	t.Helper()
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("dedicated PostgreSQL required")
	}
	root := t.TempDir()
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifact.Close(root) })
	models := store.NewStore(root)
	data := fixtureIFC("initial")
	script := "PARAMS = {'name':'generated'}\n"
	if kind == "dxf" {
		data = fixtureDXF(1)
		script = "PARAMS = {'x':2}\n"
	}
	m, err := models.CreateWithKind("synthetic."+kind, int64(len(data)), strings.NewReader(string(data)), kind)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWorkflows(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	f := &workflowFixtureExecutor{}
	w.services[kind].Executor = f
	if _, err := w.services[kind].Do(context.Background(), "PUT", "/models/"+m.ID+"/script", jsonBytes(map[string]any{"script": script})); err != nil {
		t.Fatal(err)
	}
	return w, m.ID, f, WorkflowRequest{Key: "synthetic-request", Action: "save", BaseRevision: digest([]byte(script)), Source: "ai", Note: "frozen"}
}
func workflowStep(t *testing.T, w *Workflows) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	worked, err := w.runOne(ctx)
	if err != nil || !worked {
		t.Fatal("workflow step", worked, err)
	}
}

func TestWorkflowRuntimeCheckpointRestartAndIdempotency(t *testing.T) {
	w, id, f, request := workflowFixture(t, "dxf")
	ctx := context.Background()
	r, created, err := w.Submit(ctx, id, request)
	if err != nil || !created {
		t.Fatal(r, err)
	}
	duplicate, created, err := w.Submit(ctx, id, request)
	if err != nil || created || duplicate.ID != r.ID {
		t.Fatal("duplicate intent", duplicate, err)
	}
	workflowStep(t, w)
	checkpoint, err := workflow.New(w.db).Get(ctx, id, r.ID)
	if err != nil || checkpoint.Phase != workflow.Publication || checkpoint.Candidate == nil {
		t.Fatal(checkpoint, err)
	}
	restarted, err := NewWorkflows(w.root, nil)
	if err != nil {
		t.Fatal(err)
	}
	restarted.services["dxf"].Executor = f
	workflowStep(t, restarted)
	finished, err := w.Get(ctx, id, r.ID)
	if err != nil || finished.State != workflow.Succeeded || f.calls.Load() != 1 {
		t.Fatal("geometry reran after checkpoint", finished, f.calls.Load(), err)
	}
	var count int
	if err := w.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	request.Note = "different"
	if _, _, err := w.Submit(ctx, id, request); err == nil {
		t.Fatal("idempotency key changed intent")
	}
}

func TestWorkflowRuntimeStaleAndEnvironmentDrift(t *testing.T) {
	for _, mode := range []string{"stale", "environment"} {
		t.Run(mode, func(t *testing.T) {
			w, id, f, request := workflowFixture(t, "dxf")
			ctx := context.Background()
			r, _, err := w.Submit(ctx, id, request)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "stale" {
				_, err = w.services["dxf"].Do(ctx, "PUT", "/models/"+id+"/script", jsonBytes(map[string]any{"params": map[string]any{"x": 3}}))
				if err != nil {
					t.Fatal(err)
				}
			} else {
				f.drift.Store(true)
			}
			workflowStep(t, w)
			v, err := w.Get(ctx, id, r.ID)
			want := workflow.Superseded
			if mode == "environment" {
				want = workflow.Failed
			}
			if err != nil || v.State != want || f.calls.Load() != 0 {
				t.Fatal(v, f.calls.Load(), err)
			}
		})
	}
}

func TestWorkflowRuntimePublicationRollbackAndResume(t *testing.T) {
	w, id, f, request := workflowFixture(t, "dxf")
	ctx := context.Background()
	r, _, err := w.Submit(ctx, id, request)
	if err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	if _, err = w.db.Exec(`CREATE FUNCTION reject_workflow_publish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.result IS NOT NULL AND OLD.result IS NULL THEN RAISE EXCEPTION 'synthetic publication failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_workflow_publish BEFORE UPDATE ON workflow_runs FOR EACH ROW EXECUTE FUNCTION reject_workflow_publish()`); err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	v, _ := w.Get(ctx, id, r.ID)
	if v.State != workflow.Failed {
		t.Fatal(v)
	}
	var count int
	if err := w.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&count); err != nil || count != 0 {
		t.Fatal("model published without workflow marker", count, err)
	}
	if _, err = w.db.Exec("DROP TRIGGER reject_workflow_publish ON workflow_runs; DROP FUNCTION reject_workflow_publish()"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Control(ctx, id, r.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	v, _ = w.Get(ctx, id, r.ID)
	if v.State != workflow.Succeeded || f.calls.Load() != 1 {
		t.Fatal("resume reran completed geometry", v, f.calls.Load())
	}
}

func TestWorkflowRuntimeCorruptCheckpointRepairsOnce(t *testing.T) {
	w, id, f, request := workflowFixture(t, "dxf")
	ctx := context.Background()
	r, _, err := w.Submit(ctx, id, request)
	if err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	row, _ := workflow.New(w.db).Get(ctx, id, r.ID)
	if _, err := w.db.Exec("UPDATE artifacts SET content=$1 WHERE path=$2", []byte("corrupt"), row.Candidate.Key); err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	v, _ := w.Get(ctx, id, r.ID)
	if v.Phase != workflow.Geometry {
		t.Fatal("damaged candidate accepted", v)
	}
	workflowStep(t, w)
	workflowStep(t, w)
	v, _ = w.Get(ctx, id, r.ID)
	if v.State != workflow.Succeeded || f.calls.Load() != 2 {
		t.Fatal(v, f.calls.Load())
	}
}

func TestWorkflowRuntimeCancelAndDeleteDuringGeometry(t *testing.T) {
	for _, action := range []string{"cancel", "delete"} {
		t.Run(action, func(t *testing.T) {
			w, id, f, request := workflowFixture(t, "dxf")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
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
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if action == "cancel" {
				_, err = w.Control(ctx, id, r.ID, "cancel")
			} else {
				err = w.models.DeleteContext(ctx, id)
			}
			if err != nil {
				t.Fatal(err)
			}
			close(f.gate)
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var count int
			if err := w.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&count); err != nil || count != 0 {
				t.Fatal("late geometry published", count, err)
			}
			row, err := workflow.New(w.db).Get(ctx, id, r.ID)
			if action == "cancel" {
				if err != nil || row.State != workflow.Canceled {
					t.Fatal(row, err)
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("deleted workflow revived", err)
			}
		})
	}
}

func TestWorkflowEnvironmentSealDetectsCodeChange(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(root, "bin", "python")
	if err := os.WriteFile(python, []byte("synthetic interpreter"), 0700); err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(root, "module.py")
	if err := os.WriteFile(module, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := auditEnvironment(context.Background(), python)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(module, []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := auditEnvironment(context.Background(), python)
	if err != nil || first.Digest == second.Digest {
		t.Fatal("code drift invisible without metadata change", err)
	}
}
