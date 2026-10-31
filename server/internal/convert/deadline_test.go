// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ifcviewer/server/internal/store"
)

type deadlineRunner struct{ calls atomic.Int32 }

func (r *deadlineRunner) Run(ctx context.Context, _, out string) error {
	if r.calls.Add(1) == 1 {
		if _, ok := ctx.Deadline(); !ok {
			return nil // Reproduce the missing per-job deadline without hanging.
		}
		<-ctx.Done()
		return ctx.Err()
	}
	return viewerFixture(out)
}

func TestJobDeadlineReleasesWorker(t *testing.T) {
	st := store.NewStore(t.TempDir())
	first, err := st.Create("slow.ifc", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.Create("next.ifc", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &deadlineRunner{}
	q := testQueue(t, st, runner, 1)
	q.jobTimeout = 20 * time.Millisecond // Set before enqueue; the worker is idle.
	if !q.Enqueue(first.ID) || !q.Enqueue(second.ID) {
		t.Fatal("enqueue failed")
	}
	waitStatus(t, st, first.ID, "failed")
	waitStatus(t, st, second.ID, "ready")
	got, err := st.Get(first.ID)
	if err != nil || !strings.Contains(got.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("deadline failure = %+v, %v", got, err)
	}
	if runner.calls.Load() != 2 {
		t.Fatalf("runs = %d, want 2", runner.calls.Load())
	}
}
