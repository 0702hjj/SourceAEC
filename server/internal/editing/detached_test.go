// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
)

type pausedExecutor struct{ entered, resume chan struct{} }

func (p *pausedExecutor) Run(ctx context.Context, script, kind string) (Product, error) {
	close(p.entered)
	select {
	case <-p.resume:
		return (&testExecutor{}).Run(ctx, script, kind)
	case <-ctx.Done():
		return Product{}, ctx.Err()
	}
}

func TestConcurrentSavesPublishExactlyOneVersion(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'saved'}\n"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	executors := []*pausedExecutor{{make(chan struct{}), make(chan struct{})}, {make(chan struct{}), make(chan struct{})}}
	done := make(chan error, 2)
	for _, p := range executors {
		other := New(s.Root, "ifc")
		other.Executor = p
		go func() { _, err := other.Do(ctx, "POST", "/models/"+testModel+"/script/save", nil); done <- err }()
	}
	for _, p := range executors {
		select {
		case <-p.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for _, p := range executors {
		close(p.resume)
	}
	success, conflict := 0, 0
	for range executors {
		err := <-done
		var domain *Error
		if err == nil {
			success++
		} else if errors.As(err, &domain) && domain.Status == 409 {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	versions := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
	if len(versions) != 1 || versions[0].Version != "v1" {
		t.Fatal(versions)
	}
}

func TestExecutionRejectsChangedProjectInputs(t *testing.T) {
	s, _ := testService(t, "ifc")
	write := func(key string, value any) {
		t.Helper()
		if err := artifact.WriteFile(filepath.Join(s.Root, key), jsonBytes(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("models/"+testModel+"/model.json", map[string]any{"id": testModel, "projectId": "p_test"})
	write("plans/p_test/input.json", map[string]any{"value": 1})
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'saved'}\n"})
	p := &pausedExecutor{make(chan struct{}), make(chan struct{})}
	s.Executor = p
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.Do(ctx, "POST", "/models/"+testModel+"/script/run", nil); done <- err }()
	select {
	case <-p.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	write("plans/p_test/input.json", map[string]any{"value": 2})
	close(p.resume)
	err := <-done
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 409 {
		t.Fatalf("changed inputs accepted: %v", err)
	}
}

func TestExecutionRejectsConcurrentRollbackAndBranchChange(t *testing.T) {
	for _, action := range []string{"script/rollback", "git/branches"} {
		t.Run(action, func(t *testing.T) {
			s, _ := testService(t, "ifc")
			invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'saved'}\n"})
			invoke(t, s, "POST", "script/save", nil)
			p := &pausedExecutor{make(chan struct{}), make(chan struct{})}
			s.Executor = p
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := s.Do(ctx, "POST", "/models/"+testModel+"/script/run", nil); done <- err }()
			select {
			case <-p.entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			other := New(s.Root, "ifc")
			other.Executor = &testExecutor{}
			invoke(t, other, "POST", action, map[string]any{"version": "v1", "name": "feature"})
			close(p.resume)
			err := <-done
			var domain *Error
			if !errors.As(err, &domain) || domain.Status != 409 {
				t.Fatalf("old execution overwrote %s: %v", action, err)
			}
		})
	}
}

func TestCanceledExecutionReleasesLifecycleBudget(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'saved'}\n"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 3)
	for range 3 {
		p := &pausedExecutor{make(chan struct{}), make(chan struct{})}
		other := New(s.Root, "ifc")
		other.Executor = p
		go func() { _, err := other.Do(ctx, "POST", "/models/"+testModel+"/script/run", nil); done <- err }()
		select {
		case <-p.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	_, err := s.Do(ctx, "POST", "/models/"+testModel+"/script/run", nil)
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 429 {
		t.Fatalf("unbounded lifecycle: %v", err)
	}
	cancel()
	for range 3 {
		if err := <-done; err == nil {
			t.Fatal("canceled build published")
		}
	}
	invoke(t, s, "POST", "script/run", nil)
}

func TestDeletedModelRejectsInFlightExecution(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'old'}\n"})
	p := &pausedExecutor{make(chan struct{}), make(chan struct{})}
	s.Executor = p
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.Do(ctx, "POST", "/models/"+testModel+"/script/save", nil); done <- err }()
	select {
	case <-p.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := store.NewStore(s.Root).DeleteContext(ctx, testModel); err != nil {
		t.Fatal(err)
	}
	close(p.resume)
	err := <-done
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 409 {
		t.Fatalf("deleted model accepted old build: %v", err)
	}
	if _, err = artifact.ReadFile(filepath.Join(s.Root, "uploads", testModel+".ifc")); !os.IsNotExist(err) {
		t.Fatalf("source resurrected: %v", err)
	}
}

func TestProductComparisonDoesNotConsumeGeometryCapacity(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'saved'}\n"})
	invoke(t, s, "POST", "script/save", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	done := make(chan error, 3)
	t.Cleanup(func() {
		cancel()
		for range 3 {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("execution did not cancel")
			}
		}
	})
	for range 3 {
		p := &pausedExecutor{make(chan struct{}), make(chan struct{})}
		other := New(s.Root, "ifc")
		other.Executor = p
		go func() { _, err := other.Do(ctx, "POST", "/models/"+testModel+"/script/run", nil); done <- err }()
		select {
		case <-p.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if _, err := s.Do(ctx, "POST", "/models/"+testModel+"/diff", jsonBytes(map[string]any{"base": "current", "target": "current"})); err != nil {
		t.Fatalf("stored product comparison consumed geometry admission: %v", err)
	}
	db, err := artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DELETE FROM artifacts WHERE path=$1", "models/"+testModel+"/scripts/v1.product"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Do(ctx, "POST", "/models/"+testModel+"/diff", jsonBytes(map[string]any{"base": "v1", "target": "current"}))
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 429 {
		t.Fatalf("legacy replay bypassed full geometry budget: %v", err)
	}
}

func TestExecutionReleasesDatabaseAndRejectsStaleResult(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'old'}\n"})
	p := &pausedExecutor{make(chan struct{}), make(chan struct{})}
	s.Executor = p
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.Do(ctx, "POST", "/models/"+testModel+"/script/save", nil); done <- err }()
	select {
	case <-p.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	db, err := artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if db.Stats().InUse != 0 {
		t.Errorf("execution holds database connection: %+v", db.Stats())
	}
	other := New(s.Root, "ifc")
	_, err = other.Do(ctx, "PUT", "/models/"+testModel+"/script", jsonBytes(map[string]any{"script": "PARAMS = {'name':'new'}\n"}))
	close(p.resume)
	if err != nil {
		t.Fatalf("stage blocked during execution: %v", err)
	}
	err = <-done
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 409 {
		t.Fatalf("stale execution: %v", err)
	}
	versions := invoke(t, other, "GET", "versions", nil).(map[string]any)["versions"].([]version)
	if len(versions) != 0 {
		t.Fatalf("stale save published: %v", versions)
	}
}
