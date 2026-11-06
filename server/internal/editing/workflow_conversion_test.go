// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/workflow"
)

type workflowPreviewRunner struct{}

func (workflowPreviewRunner) Run(ctx context.Context, input, out string) error {
	for name, data := range map[string]string{"model.xkt": "synthetic-xkt", "metadata.json": "{}"} {
		if err := os.WriteFile(filepath.Join(out, name), []byte(data), 0600); err != nil {
			return err
		}
	}
	return ctx.Err()
}
func TestWorkflowConversionRestartAndCancel(t *testing.T) {
	for _, mode := range []string{"restart", "cancel", "superseded"} {
		t.Run(mode, func(t *testing.T) {
			w, id, f, request := workflowFixture(t, "ifc")
			ctx := context.Background()
			r, _, err := w.Submit(ctx, id, request)
			if err != nil {
				t.Fatal(err)
			}
			workflowStep(t, w)
			workflowStep(t, w)
			published, err := w.Get(ctx, id, r.ID)
			if err != nil || published.Phase != workflow.Conversion || len(published.Result) == 0 {
				t.Fatal(published, err)
			}
			if mode == "cancel" {
				if _, err = w.Control(ctx, id, r.ID, "cancel"); err != nil {
					t.Fatal(err)
				}
				v, _ := w.Get(ctx, id, r.ID)
				if v.State != workflow.Canceled || len(v.Result) == 0 {
					t.Fatal("cancel erased committed result", v)
				}
			} else {
				q := convert.NewQueue(w.models, workflowPreviewRunner{}, 1)
				defer func() {
					q.Close()
					join, stop := context.WithTimeout(ctx, 5*time.Second)
					defer stop()
					if !q.WaitIdle(join) {
						t.Error("converter did not stop")
					}
				}()
				restarted, err := NewWorkflows(w.root, q)
				if err != nil {
					t.Fatal(err)
				}
				restarted.services["ifc"].Executor = f
				if mode == "superseded" {
					if err = artifact.WriteFile(w.models.IFCPath(id)+".build.json", []byte(`{"revision":"changed"}`), 0600); err != nil {
						t.Fatal(err)
					}
				}
				workflowStep(t, restarted)
				v, _ := w.Get(ctx, id, r.ID)
				want := workflow.Succeeded
				if mode == "superseded" {
					want = workflow.Superseded
				}
				if v.State != want {
					t.Fatal(v)
				}
			}
			var count int
			if err = w.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&count); err != nil || count != 1 {
				t.Fatal(count, err)
			}
			if mode != "superseded" && f.calls.Load() != 1 {
				t.Fatal("conversion reran geometry", f.calls.Load())
			}
		})
	}
}
func TestWorkflowRealSandboxPinnedEnvironment(t *testing.T) {
	if os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" {
		t.Skip("external geometry integration disabled")
	}
	w, id, _, request := workflowFixture(t, "dxf")
	script := `PARAMS = {"x": 3.0}
import ezdxf
import sys
doc = ezdxf.new()
doc.modelspace().add_line((0, 0), (PARAMS["x"], 1))
doc.saveas(sys.argv[1])
`
	s := w.services["dxf"]
	s.Executor = NewSandbox()
	if _, err := s.Do(context.Background(), "PUT", "/models/"+id+"/script", jsonBytes(map[string]any{"script": script})); err != nil {
		t.Fatal(err)
	}
	request.BaseRevision = digest([]byte(script))
	r, _, err := w.Submit(context.Background(), id, request)
	if err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	workflowStep(t, w)
	v, err := w.Get(context.Background(), id, r.ID)
	if err != nil || v.State != workflow.Succeeded {
		t.Fatal(v, err)
	}
}

func TestWorkflowRealIFCConverterRecovery(t *testing.T) {
	if os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" || os.Getenv("SOURCEAEC_TEST_CONVERTER") != "1" {
		t.Skip("external geometry and converter integration disabled")
	}
	w, id, _, request := workflowFixture(t, "ifc")
	converter, err := filepath.Abs("../../../converter")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(converter, "test/fixtures/wall-with-opening-and-window.ifc"))
	if err != nil {
		t.Fatal(err)
	}
	script := "PARAMS = {}\nimport sys\nimport ifcopenshell\nmodel = ifcopenshell.file.from_string(" + pythonLiteral(string(data)) + ")\nmodel.write(sys.argv[1])\n"
	s := w.services["ifc"]
	s.Executor = NewSandbox()
	if _, err = s.Do(context.Background(), "PUT", "/models/"+id+"/script", jsonBytes(map[string]any{"script": script})); err != nil {
		t.Fatal(err)
	}
	request.BaseRevision = digest([]byte(script))
	r, _, err := w.Submit(context.Background(), id, request)
	if err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	workflowStep(t, w)
	// Recreate the runtime with an empty converter queue after publication.
	q := convert.NewQueue(w.models, convert.ExecRunner{NodeBin: "node", Script: filepath.Join(converter, "convert.js")}, 1)
	defer func() {
		q.Close()
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if !q.WaitIdle(ctx) {
			t.Error("converter shutdown")
		}
	}()
	restarted, err := NewWorkflows(w.root, q)
	if err != nil {
		t.Fatal(err)
	}
	workflowStep(t, restarted)
	v, err := restarted.Get(context.Background(), id, r.ID)
	if err != nil || v.State != workflow.Succeeded {
		t.Fatal(v, err)
	}
	for _, name := range []string{"model.xkt", "metadata.json"} {
		path, err := w.models.ViewerPath(context.Background(), id, name)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			t.Fatal(name, err)
		}
	}
	var count int
	if err = w.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
