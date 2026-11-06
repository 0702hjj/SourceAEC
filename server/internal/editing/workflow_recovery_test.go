// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/workflow"
)

type crashWorkflowExecutor struct {
	workflowFixtureExecutor
	root  string
	block bool
}

func (f *crashWorkflowExecutor) RunFrozen(ctx context.Context, script, kind string, seal EnvironmentSeal) (Product, error) {
	file, err := os.OpenFile(filepath.Join(f.root, "geometry-count"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return Product{}, err
	}
	_, err = file.WriteString("geometry\n")
	if e := file.Close(); err == nil {
		err = e
	}
	if err != nil {
		return Product{}, err
	}
	if f.block {
		fmt.Println("WORKFLOW_READY")
		<-ctx.Done()
		return Product{}, ctx.Err()
	}
	return f.workflowFixtureExecutor.RunFrozen(ctx, script, kind, seal)
}

// Run in a genuinely separate process; the parent kills it at committed phase
// boundaries or inside geometry, then recovers through the ordinary worker.
func TestWorkflowCrashProcessHelper(t *testing.T) {
	root := os.Getenv("SOURCEAEC_WORKFLOW_CRASH_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	w, err := NewWorkflows(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	mode := os.Getenv("SOURCEAEC_WORKFLOW_CRASH_MODE")
	kind := os.Getenv("SOURCEAEC_WORKFLOW_CRASH_KIND")
	if kind == "" {
		kind = "dxf"
	}
	w.services[kind].Executor = &crashWorkflowExecutor{root: root, block: mode == "geometry"}
	if worked, err := w.runOne(context.Background()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	fmt.Println("WORKFLOW_READY")
	select {}
}
func TestWorkflowProcessCrashMatrix(t *testing.T) {
	for _, mode := range []string{"geometry", "checkpoint", "published", "conversion"} {
		t.Run(mode, func(t *testing.T) {
			kind := "dxf"
			if mode == "conversion" {
				kind = "ifc"
			}
			w, id, _, request := workflowFixture(t, kind)
			ctx := context.Background()
			r, _, err := w.Submit(ctx, id, request)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "published" || mode == "conversion" {
				w.services[kind].Executor = &crashWorkflowExecutor{root: w.root}
				workflowStep(t, w)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestWorkflowCrashProcessHelper$", "-test.timeout=45s")
			cmd.Env = append(os.Environ(), "SOURCEAEC_WORKFLOW_CRASH_ROOT="+w.root, "SOURCEAEC_WORKFLOW_CRASH_MODE="+mode, "SOURCEAEC_WORKFLOW_CRASH_KIND="+kind)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr strings.Builder
			cmd.Stderr = &stderr
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill() }()
			ready := make(chan bool, 1)
			go func() {
				scan := bufio.NewScanner(stdout)
				for scan.Scan() {
					if scan.Text() == "WORKFLOW_READY" {
						ready <- true
						return
					}
				}
				ready <- false
			}()
			select {
			case ok := <-ready:
				if !ok {
					_ = cmd.Wait()
					t.Fatal("child failed before crash boundary", stderr.String())
				}
			case <-time.After(30 * time.Second):
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal("child boundary timeout", stderr.String())
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			// Move DB time boundaries explicitly; never wait for a lease using sleeps.
			if _, err = w.db.Exec("UPDATE workflow_runs SET lease_until=clock_timestamp()-interval '1 second',available_at=clock_timestamp()-interval '1 second' WHERE id=$1 AND state='running'", r.ID); err != nil {
				t.Fatal(err)
			}
			if err = artifact.Close(w.root); err != nil {
				t.Fatal(err)
			}
			if err = artifact.Open(w.root); err != nil {
				t.Fatal(err)
			}
			var preview PreviewQueue
			if mode == "conversion" {
				q := convert.NewQueue(w.models, workflowPreviewRunner{}, 1)
				defer func() {
					q.Close()
					join, stop := context.WithTimeout(ctx, 5*time.Second)
					defer stop()
					if !q.WaitIdle(join) {
						t.Error("converter shutdown")
					}
				}()
				preview = q
			}
			restarted, err := NewWorkflows(w.root, preview)
			if err != nil {
				t.Fatal(err)
			}
			restarted.services[kind].Executor = &crashWorkflowExecutor{root: w.root}
			for i := 0; i < 4; i++ {
				state, e := restarted.Get(ctx, id, r.ID)
				if e != nil {
					t.Fatal(e)
				}
				if state.State == workflow.Succeeded {
					break
				}
				workflowStep(t, restarted)
			}
			state, err := restarted.Get(ctx, id, r.ID)
			if err != nil || state.State != workflow.Succeeded {
				t.Fatal(state, err)
			}
			data, err := os.ReadFile(filepath.Join(w.root, "geometry-count"))
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if mode == "geometry" {
				want = 2
			}
			if count := strings.Count(string(data), "geometry\n"); count != want {
				t.Fatalf("geometry executions=%d want=%d", count, want)
			}
			var count int
			if err = restarted.db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1", id).Scan(&count); err != nil || count != 1 {
				t.Fatal("duplicate/missing publication", count, err)
			}
		})
	}
}

func TestWorkflowScannerRecoversLostWakeupAndStops(t *testing.T) {
	w, id, _, request := workflowFixture(t, "dxf")
	r, _, err := w.Submit(context.Background(), id, request)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewWorkflows(w.root, nil)
	if err != nil {
		t.Fatal(err)
	}
	restarted.services["dxf"].Executor = &workflowFixtureExecutor{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restarted.Start(ctx)
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := restarted.Get(ctx, id, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.State == workflow.Succeeded {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("scanner lost committed work", state)
		case <-ticker.C:
		}
	}
	cancel()
	join, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if !restarted.Wait(join) {
		t.Fatal("worker/heartbeat did not stop")
	}
}
