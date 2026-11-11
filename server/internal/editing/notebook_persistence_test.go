// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/notebookcache"
	"ifcviewer/server/internal/store"
)

type observedNotebook struct {
	*Sandbox
	last Product
}

func (s *observedNotebook) Run(ctx context.Context, source, kind string) (Product, error) {
	product, err := s.Sandbox.Run(ctx, source, kind)
	s.last = product
	return product, err
}
func TestNotebookRealPublicationRestartCorruptionAndDelete(t *testing.T) {
	sandbox := notebookSandbox(t)
	s, _ := testService(t, "dxf")
	if err := artifact.WriteFile(filepath.Join(s.Root, "models", testModel, "model.json"), []byte(`{"projectId":"p_notebook"}`), 0600); err != nil {
		t.Fatal(err)
	}
	input := "skill-work/p_notebook/part.py"
	if err := artifact.WriteFile(filepath.Join(s.Root, input), []byte(notebookModel), 0600); err != nil {
		t.Fatal(err)
	}
	source := strings.ReplaceAll(notebookWrapper, "skill-work/part.py", input)
	observer := &observedNotebook{Sandbox: sandbox}
	s.Executor = observer
	invoke(t, s, "PUT", "script", map[string]any{"script": source})
	invoke(t, s, "POST", "script/save", nil)
	if cachedCells(notebookReportOf(t, observer.last)) != 0 {
		t.Fatal("first model run restored unrelated cache")
	}
	db, err := artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = notebookcache.New(db).Load(context.Background(), testModel); err != nil {
		t.Fatal("successful publication did not commit cache", err)
	}
	if err = artifact.Close(s.Root); err != nil {
		t.Fatal(err)
	}
	if err = artifact.Open(s.Root); err != nil {
		t.Fatal(err)
	}
	s = New(s.Root, "dxf")
	s.Executor = observer
	invoke(t, s, "POST", "script/run", nil)
	if cachedCells(notebookReportOf(t, observer.last)) == 0 {
		t.Fatal("restart did not restore persisted SDK cache")
	}
	db, err = artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE notebook_generations SET payload='corrupt'::bytea WHERE model_id=$1`, testModel); err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "POST", "script/run", nil)
	if cachedCells(notebookReportOf(t, observer.last)) != 0 {
		t.Fatal("corrupt cache was reused")
	}
	if _, err = notebookcache.New(db).Load(context.Background(), testModel); err != nil {
		t.Fatal("cold rerun did not repair cache", err)
	}
	if err = store.NewStore(s.Root).DeleteContext(context.Background(), testModel); err != nil {
		t.Fatal(err)
	}
	if _, err = notebookcache.New(db).Load(context.Background(), testModel); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("model deletion retained notebook head", err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM notebook_generations WHERE model_id=$1`, testModel).Scan(&count); err != nil || count != 0 {
		t.Fatal("model deletion retained notebook generation", count, err)
	}
}

type cachedWorkflowFixture struct {
	workflowFixtureExecutor
	value notebookcache.Snapshot
}

func (f *cachedWorkflowFixture) RunFrozen(ctx context.Context, source, kind string, seal EnvironmentSeal) (Product, error) {
	product, err := f.workflowFixtureExecutor.RunFrozen(ctx, source, kind, seal)
	product.NotebookCache = &f.value
	return product, err
}
func TestNotebookWorkflowCheckpointAtomicAndDeletion(t *testing.T) {
	w, id, _, request := workflowFixture(t, "dxf")
	ctx := context.Background()
	root := t.TempDir()
	prefix := "skill-work/__marimo__"
	if err := os.MkdirAll(filepath.Join(root, prefix), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, prefix, "opaque.pkl"), []byte("synthetic opaque SDK cache"), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := notebookcache.Pack(ctx, root, []string{prefix}, digest([]byte("compatibility")))
	if err != nil {
		t.Fatal(err)
	}
	f := &cachedWorkflowFixture{value: value}
	w.services["dxf"].Executor = f
	run, _, err := w.Submit(ctx, id, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.db.Exec(`CREATE FUNCTION reject_notebook_checkpoint() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.phase='publication' THEN RAISE EXCEPTION 'synthetic checkpoint failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_notebook_checkpoint BEFORE UPDATE ON workflow_runs FOR EACH ROW EXECUTE FUNCTION reject_notebook_checkpoint()`); err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	if _, err = notebookcache.New(w.db).Load(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("failed checkpoint committed cache", err)
	}
	if _, err = w.db.Exec(`DROP TRIGGER reject_notebook_checkpoint ON workflow_runs; DROP FUNCTION reject_notebook_checkpoint()`); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Control(ctx, id, run.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	if _, err = notebookcache.New(w.db).Load(ctx, id); err != nil {
		t.Fatal("geometry checkpoint did not commit cache", err)
	}
	workflowStep(t, w)
	if err = w.models.DeleteContext(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = notebookcache.New(w.db).Load(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("deleted model cache remains", err)
	}
}

func TestNotebookWorkflowCancelOrDeleteRejectsLateCache(t *testing.T) {
	for _, action := range []string{"cancel", "delete"} {
		t.Run(action, func(t *testing.T) {
			w, id, _, request := workflowFixture(t, "dxf")
			ctx := context.Background()
			root := t.TempDir()
			prefix := "skill-work/__marimo__"
			if err := os.MkdirAll(filepath.Join(root, prefix), 0700); err != nil {
				t.Fatal(err)
			}
			value, err := notebookcache.Pack(ctx, root, []string{prefix}, digest([]byte("identity")))
			if err != nil {
				t.Fatal(err)
			}
			f := &cachedWorkflowFixture{value: value}
			f.entered = make(chan struct{})
			f.gate = make(chan struct{})
			w.services["dxf"].Executor = f
			run, _, err := w.Submit(ctx, id, request)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := w.runOne(ctx); done <- err }()
			select {
			case <-f.entered:
			case <-time.After(10 * time.Second):
				t.Fatal("geometry not entered")
			}
			if action == "cancel" {
				_, err = w.Control(ctx, id, run.ID, "cancel")
			} else {
				err = w.models.DeleteContext(ctx, id)
			}
			if err != nil {
				t.Fatal(err)
			}
			close(f.gate)
			select {
			case err = <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("late geometry did not finish")
			}
			if _, err = notebookcache.New(w.db).Load(ctx, id); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("late cache escaped cancel/delete fence", err)
			}
		})
	}
}

func TestNotebookTimeoutDoesNotPublishPartialCache(t *testing.T) {
	sandbox := notebookSandbox(t)
	sandbox.Timeout = 10 * time.Second
	s, _ := testService(t, "dxf")
	s.Executor = sandbox
	input := "skill-work/p_notebook/part.py"
	if err := artifact.WriteFile(filepath.Join(s.Root, "models", testModel, "model.json"), []byte(`{"projectId":"p_notebook"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := artifact.WriteFile(filepath.Join(s.Root, input), []byte(notebookModel), 0600); err != nil {
		t.Fatal(err)
	}
	header := strings.Split(notebookWrapper, "PARAMS = {}")[0]
	header = strings.ReplaceAll(header, "skill-work/part.py", input)
	source := header + `PARAMS = {}
from pathlib import Path
Path('/inputs/skill-work/p_notebook/__marimo__/partial.pkl').write_bytes(b'partial SDK cache')
while True:
    pass
`
	invoke(t, s, "PUT", "script", map[string]any{"script": source})
	_, err := s.Do(context.Background(), "POST", "/models/"+testModel+"/script/save", nil)
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 504 {
		t.Fatal("expected notebook timeout", err)
	}
	db, err := artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = notebookcache.New(db).Load(context.Background(), testModel); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("timeout accepted partial cache", err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM version_events WHERE model_id=$1`, testModel).Scan(&count); err != nil || count != 0 {
		t.Fatal("timeout committed a version", count, err)
	}
}
