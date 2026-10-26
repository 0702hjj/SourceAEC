// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/metrics"
)

type blockedExecutor struct{ started chan struct{} }

func (e *blockedExecutor) Run(ctx context.Context, _, _ string) (Product, error) {
	close(e.started)
	<-ctx.Done()
	return Product{}, ctx.Err()
}

func startBlockedRun(t *testing.T, s *Service) {
	t.Helper()
	executor := &blockedExecutor{started: make(chan struct{})}
	s.Executor = executor
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan error, 1)
	go func() {
		_, err := s.Do(ctx, "POST", "/models/"+testModel+"/script/run", nil)
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err == nil {
				t.Error("canceled execution unexpectedly succeeded")
			}
		case <-time.After(5 * time.Second):
			t.Error("canceled execution did not finish")
		}
	})
	select {
	case <-executor.started:
	case <-ctx.Done():
		t.Fatal("execution did not start")
	}
}

func TestIndependentNamespacesDoNotShareModelLock(t *testing.T) {
	t.Setenv("VIEWER_STORAGE_NAMESPACE", "")
	first, _ := testService(t, "ifc")
	second, _ := testService(t, "ifc")
	for _, s := range []*Service{first, second} {
		invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'base'}\n"})
	}
	startBlockedRun(t, first)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := second.Do(ctx, "GET", "/models/"+testModel+"/script", nil); err != nil {
		t.Fatalf("unrelated namespace blocked by identical model ID: %v", err)
	}
}

func TestBlockedModelAllowsAnotherModel(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'base'}\n"})
	const otherModel = "m_0000000000000002"
	if err := artifact.WriteFile(filepath.Join(s.Root, "uploads", otherModel+".ifc"), fixtureIFC("other"), 0644); err != nil {
		t.Fatal(err)
	}
	startBlockedRun(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	other := New(s.Root, "ifc")
	if _, err := other.Do(ctx, "PUT", "/models/"+otherModel+"/script", jsonBytes(map[string]any{"script": "PARAMS = {'name':'other'}\n"})); err != nil {
		t.Fatalf("unrelated model blocked: %v", err)
	}
}

func TestHotModelWaitDoesNotOccupyDatabasePool(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'base'}\n"})
	const otherModel = "m_0000000000000002"
	if err := artifact.WriteFile(filepath.Join(s.Root, "uploads", otherModel+".ifc"), fixtureIFC("other"), 0644); err != nil {
		t.Fatal(err)
	}
	db, err := artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(2)
	startBlockedRun(t, s)
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	done := make(chan error, 1)
	otherInstance := New(s.Root, "ifc")
	go func() {
		_, err := otherInstance.Do(waitCtx, "GET", "/models/"+testModel+"/script", nil)
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err == nil {
				t.Error("waiting request unexpectedly completed")
			}
		case <-time.After(5 * time.Second):
			t.Error("waiting request did not cancel")
		}
	})
	// A condition confirms the contender reached admission; no timing sleep.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for metrics.EditModelWaiting.Value() == 0 {
		select {
		case <-ticker.C:
		case <-waitCtx.Done():
			t.Fatal("contender did not reach model admission")
		}
	}
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	_, err = otherInstance.Do(ctx, "PUT", "/models/"+otherModel+"/script", jsonBytes(map[string]any{"script": "PARAMS = {'name':'other'}\n"}))
	if err != nil {
		t.Fatalf("hot-model waiter exhausted pool for unrelated model: %v", err)
	}
}
