// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editruntime"
)

func TestConcurrentInstancesRejectLostUpdate(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'base'}\n"})
	revision := invoke(t, s, "GET", "script", nil).(map[string]any)["revision"]
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, name := range []string{"first", "second"} {
		other := New(s.Root, "ifc")
		workers.Add(1)
		go func(name string) {
			defer workers.Done()
			<-start
			_, err := other.Do(ctx, "PUT", "/models/"+testModel+"/script", jsonBytes(map[string]any{
				"script": "PARAMS = {'name':'" + name + "'}\n", "baseRevision": revision,
			}))
			results <- err
		}(name)
	}
	close(start)
	workers.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		var domain *Error
		switch {
		case err == nil:
			success++
		case errors.As(err, &domain) && domain.Status == 409:
			conflict++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d; expected one of each", success, conflict)
	}
	state := invoke(t, s, "GET", "script", nil).(map[string]any)
	if state["staged"] != 2 || state["revision"] == revision {
		t.Fatalf("lost or duplicated stage: %v", state)
	}
}

func TestCanceledModelLockWaitReleasesConnection(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'base'}\n"})
	runtime := editruntime.NewEmbedded()
	defer runtime.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	value, err := runtime.Handle(editruntime.Request{Context: ctx, Root: s.Root, Op: "begin"})
	if err != nil {
		t.Fatal(err)
	}
	tx := value.(string)
	if _, err = runtime.Handle(editruntime.Request{Context: ctx, Root: s.Root, Tx: tx, Op: "lock", Args: jsonBytes(testModel)}); err != nil {
		t.Fatal(err)
	}
	waitCtx, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stop()
	_, err = s.Do(waitCtx, "PUT", "/models/"+testModel+"/script", jsonBytes(map[string]any{"params": map[string]any{"name": "waiting"}}))
	if err == nil || !errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
		t.Fatalf("lock wait must honor deadline: %v (%v)", err, waitCtx.Err())
	}
	if _, err = runtime.Handle(editruntime.Request{Root: s.Root, Tx: tx, Op: "rollback"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Do(ctx, "GET", "/models/"+testModel+"/script", nil); err != nil {
		t.Fatalf("request after canceled waiter failed: %v", err)
	}
	db, err := artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if active := db.Stats().InUse; active != 0 {
		t.Fatalf("connections retained after cancellation: %d", active)
	}
}
