// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/workflow"
)

func TestWorkflowProcessCrashTransactionBoundaries(t *testing.T) {
	cases := []struct {
		name, kind, table, timing, condition string
		prepare, geometry, conversions       int
	}{
		{name: "before-claim", kind: "dxf", geometry: 1},
		{name: "after-claim", kind: "dxf", geometry: 1},
		{name: "claim-before-commit", kind: "dxf", table: "workflow_runs", timing: "AFTER UPDATE", condition: "NEW.id='%s' AND NEW.state='running' AND OLD.state='pending'", geometry: 1},
		{name: "product-ready", kind: "dxf", geometry: 2},
		{name: "checkpoint-before-commit", kind: "dxf", table: "workflow_runs", timing: "AFTER UPDATE", condition: "NEW.id='%s' AND NEW.phase='publication' AND OLD.phase='geometry'", geometry: 2},
		{name: "head-before-update", kind: "dxf", table: "version_heads", timing: "BEFORE UPDATE", condition: "NEW.model_id='%s' AND NEW.ref IS DISTINCT FROM OLD.ref", prepare: 1, geometry: 1},
		{name: "head-after-update", kind: "dxf", table: "version_heads", timing: "AFTER UPDATE", condition: "NEW.model_id='%s' AND NEW.ref IS DISTINCT FROM OLD.ref", prepare: 1, geometry: 1},
		{name: "publication-before-commit", kind: "dxf", table: "workflow_runs", timing: "AFTER UPDATE", condition: "NEW.id='%s' AND NEW.state='succeeded'", prepare: 1, geometry: 1},
		{name: "conversion-during-publication", kind: "ifc", table: "artifacts", timing: "AFTER INSERT OR UPDATE", condition: "NEW.path='models/%s/viewer.json'", prepare: 2, geometry: 1, conversions: 2},
		{name: "conversion-after-publication", kind: "ifc", table: "workflow_runs", timing: "BEFORE UPDATE", condition: "NEW.id='%s' AND NEW.state='succeeded'", prepare: 2, geometry: 1, conversions: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			worker, id, _, request := workflowFixture(t, tc.kind)
			ctx := context.Background()
			run, _, err := worker.Submit(ctx, id, request)
			if err != nil {
				t.Fatal(err)
			}
			worker.services[tc.kind].Executor = &crashWorkflowExecutor{root: worker.root}
			for i := 0; i < tc.prepare; i++ {
				workflowStep(t, worker)
			}
			var oldHead string
			if err := worker.db.QueryRow("SELECT ref FROM version_heads WHERE model_id=$1", id).Scan(&oldHead); err != nil {
				t.Fatal(err)
			}
			var pause *workflowBoundaryPause
			if tc.table != "" {
				key := run.ID
				if tc.table == "version_heads" || tc.table == "artifacts" {
					key = id
				}
				pause = installWorkflowBoundaryPause(t, worker.db, run.ID, tc.table, tc.timing, fmt.Sprintf(tc.condition, key))
			}
			child := startBoundaryChild(t, worker.root, tc.name, tc.kind)
			if pause != nil {
				pause.await(t, child)
			} else {
				child.await(t)
			}
			child.kill()
			if pause != nil {
				pause.stop(t)
			}
			if t.Failed() {
				t.FailNow()
			}
			var events int
			if err := worker.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&events); err != nil {
				t.Fatal(err)
			}
			wantEvents := 0
			if tc.kind == "ifc" {
				wantEvents = 1
			}
			if events != wantEvents {
				t.Fatalf("partial/duplicate committed versions: got %d want %d", events, wantEvents)
			}
			if tc.kind == "dxf" {
				var head string
				if err := worker.db.QueryRow("SELECT ref FROM version_heads WHERE model_id=$1", id).Scan(&head); err != nil || head != oldHead {
					t.Fatalf("uncommitted head visible: %s %v", head, err)
				}
			}
			if tc.name == "conversion-during-publication" {
				if _, err := worker.models.ViewerPath(ctx, id, "model.xkt"); err == nil {
					t.Fatal("half-published preview became visible")
				}
			}
			if tc.name == "conversion-after-publication" {
				if _, err := worker.models.ViewerPath(ctx, id, "model.xkt"); err != nil {
					t.Fatal("committed preview lost", err)
				}
			}
			if _, err := worker.db.Exec("UPDATE workflow_runs SET lease_until=clock_timestamp()-interval '1 second',available_at=clock_timestamp()-interval '1 second' WHERE id=$1 AND state='running'", run.ID); err != nil {
				t.Fatal(err)
			}
			if err := artifact.Close(worker.root); err != nil {
				t.Fatal(err)
			}
			if err := artifact.Open(worker.root); err != nil {
				t.Fatal(err)
			}
			var preview PreviewQueue
			if tc.kind == "ifc" {
				queue := convert.NewQueue(worker.models, boundaryConversionRunner{worker.root}, 1)
				defer func() {
					queue.Close()
					join, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					if !queue.WaitIdle(join) {
						t.Error("preview workers did not stop")
					}
				}()
				preview = queue
			}
			restarted, err := NewWorkflows(worker.root, preview)
			if err != nil {
				t.Fatal(err)
			}
			restarted.services[tc.kind].Executor = &crashWorkflowExecutor{root: worker.root}
			for step := 0; step < 4; step++ {
				state, err := restarted.Get(ctx, id, run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if state.State == workflow.Succeeded {
					break
				}
				workflowStep(t, restarted)
			}
			state, err := restarted.Get(ctx, id, run.ID)
			if err != nil || state.State != workflow.Succeeded {
				t.Fatal(state, err)
			}
			if err := restarted.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&events); err != nil || events != 1 {
				t.Fatal("missing or duplicated version after recovery", events, err)
			}
			assertBoundaryCount(t, worker.root, "geometry-count", "geometry\n", tc.geometry)
			if tc.conversions > 0 {
				assertBoundaryCount(t, worker.root, "conversion-count", "conversion\n", tc.conversions)
			}
		})
	}
}
func assertBoundaryCount(t *testing.T, root, file, marker string, expected int) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, file))
	if err != nil || strings.Count(string(data), marker) != expected {
		t.Fatalf("%s count=%d want=%d error=%v", file, strings.Count(string(data), marker), expected, err)
	}
}
