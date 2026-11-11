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
	"ifcviewer/server/internal/store"
	"ifcviewer/server/internal/workflow"
)

func boundaryReady() { fmt.Println("WORKFLOW_BOUNDARY_READY"); select {} }

type boundaryGeometryExecutor struct {
	crashWorkflowExecutor
	productReady bool
}

func (e *boundaryGeometryExecutor) RunFrozen(ctx context.Context, script, kind string, seal EnvironmentSeal) (Product, error) {
	product, err := e.crashWorkflowExecutor.RunFrozen(ctx, script, kind, seal)
	if err == nil && e.productReady {
		if err = os.WriteFile(filepath.Join(e.root, "completed-geometry.product"), product.Data, 0600); err != nil {
			return Product{}, err
		}
		boundaryReady()
	}
	return product, err
}

type boundaryConversionRunner struct{ root string }

func (r boundaryConversionRunner) Run(ctx context.Context, input, output string) error {
	file, err := os.OpenFile(filepath.Join(r.root, "conversion-count"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString("conversion\n")
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return (workflowPreviewRunner{}).Run(ctx, input, output)
}

func TestWorkflowBoundaryProcessHelper(t *testing.T) {
	root := os.Getenv("SOURCEAEC_BOUNDARY_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	mode, kind := os.Getenv("SOURCEAEC_BOUNDARY_MODE"), os.Getenv("SOURCEAEC_BOUNDARY_KIND")
	var queue PreviewQueue
	if kind == "ifc" {
		queue = convert.NewQueue(store.NewStore(root), boundaryConversionRunner{root}, 1)
	}
	worker, err := NewWorkflows(root, queue)
	if err != nil {
		t.Fatal(err)
	}
	worker.services[kind].Executor = &boundaryGeometryExecutor{crashWorkflowExecutor: crashWorkflowExecutor{root: root}, productReady: mode == "product-ready"}
	if mode == "before-claim" {
		boundaryReady()
	}
	if mode == "after-claim" {
		err := worker.transaction(context.Background(), func(state *workflow.Store) error {
			_, found, err := state.Claim(context.Background(), "crash-boundary-worker", 30*time.Second)
			if err == nil && !found {
				return fmt.Errorf("no run to claim")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		boundaryReady()
	}
	worked, err := worker.runOne(context.Background())
	if err != nil || !worked {
		t.Fatal(worked, err)
	}
	t.Fatal("worker crossed expected interruption boundary")
}

type boundaryChild struct {
	command *exec.Cmd
	stderr  strings.Builder
	ready   chan bool
	waited  bool
}

func startBoundaryChild(t *testing.T, root, mode, kind string) *boundaryChild {
	t.Helper()
	child := &boundaryChild{ready: make(chan bool, 1)}
	child.command = exec.Command(os.Args[0], "-test.run=^TestWorkflowBoundaryProcessHelper$", "-test.timeout=45s")
	child.command.Env = append(os.Environ(), "SOURCEAEC_BOUNDARY_ROOT="+root, "SOURCEAEC_BOUNDARY_MODE="+mode, "SOURCEAEC_BOUNDARY_KIND="+kind)
	child.command.Stderr = &child.stderr
	stdout, err := child.command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.kill() })
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if scanner.Text() == "WORKFLOW_BOUNDARY_READY" {
				child.ready <- true
				return
			}
		}
		child.ready <- false
	}()
	return child
}
func (c *boundaryChild) await(t *testing.T) {
	t.Helper()
	select {
	case ready := <-c.ready:
		if !ready {
			c.kill()
			t.Fatal("child exited before boundary", c.stderr.String())
		}
	case <-time.After(15 * time.Second):
		c.kill()
		t.Fatal("child boundary deadline", c.stderr.String())
	}
}
func (c *boundaryChild) kill() {
	if c.waited {
		return
	}
	_ = c.command.Process.Kill()
	_ = c.command.Wait()
	c.waited = true
}
