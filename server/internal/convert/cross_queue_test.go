// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"context"
	"errors"
	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
	"strings"
	"testing"
	"time"
)

type orderedRunner struct {
	entered, resume chan struct{}
	fail            bool
}

func (r *orderedRunner) Run(ctx context.Context, _, out string) error {
	close(r.entered)
	select {
	case <-r.resume:
	case <-ctx.Done():
		return ctx.Err()
	}
	if r.fail {
		return errors.New("late duplicate failed")
	}
	return viewerFixture(out)
}
func TestConcurrentQueuesCannotOverwriteMatchingReadyPreview(t *testing.T) {
	st := store.NewStore(t.TempDir())
	if err := artifact.Open(st.DataDir); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(st.DataDir)
	model, err := st.Create("synthetic.ifc", 3, strings.NewReader("ifc"))
	if err != nil {
		t.Fatal(err)
	}
	first := &orderedRunner{make(chan struct{}), make(chan struct{}), false}
	second := &orderedRunner{make(chan struct{}), make(chan struct{}), true}
	a, b := NewQueue(st, first, 1), NewQueue(store.NewStore(st.DataDir), second, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer func() {
		a.Close()
		b.Close()
		if !a.WaitIdle(ctx) || !b.WaitIdle(ctx) {
			t.Error("queues did not drain")
		}
	}()
	if !a.Enqueue(model.ID) || !b.Enqueue(model.ID) {
		t.Fatal("queues did not accept jobs")
	}
	for _, r := range []*orderedRunner{first, second} {
		select {
		case <-r.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	wait := func(q *Queue) {
		t.Helper()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			pending, _ := q.Depth()
			if pending == 0 {
				return
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	close(first.resume)
	wait(a)
	before, err := st.Get(model.ID)
	if err != nil || before.Status != "ready" {
		t.Fatal(before, err)
	}
	close(second.resume)
	wait(b)
	after, err := st.Get(model.ID)
	if err != nil || after.Status != "ready" || after.PreviewGeneration != before.PreviewGeneration {
		t.Fatal("late queue overwrote ready", after, err)
	}
}
