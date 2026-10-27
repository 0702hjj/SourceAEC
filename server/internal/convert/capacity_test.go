// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"context"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
)

func TestFullQueueRejectsWithoutBlockingShutdown(t *testing.T) {
	st := store.NewStore(t.TempDir())
	// No workers keeps capacity deterministic without timing assumptions.
	q := testQueue(t, st, fakeRunner{}, 0)
	for i := 0; i < cap(q.jobs); i++ {
		m, err := st.Create("synthetic.ifc", 1, strings.NewReader("x"))
		if err != nil || !q.Enqueue(m.ID) {
			t.Fatal("queue rejected before capacity", err)
		}
	}
	m, err := st.Create("overflow.ifc", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan bool, 1)
	go func() { done <- q.EnqueueIfStale(m.ID) }()
	select {
	case accepted := <-done:
		if accepted {
			t.Fatal("full queue accepted request")
		}
	case <-time.After(time.Second):
		t.Fatal("full queue blocked producer")
	}
	model, err := st.Get(m.ID)
	if err != nil || model.Status != "failed" || model.Error == "" {
		t.Fatalf("rejected work stranded in converting: %+v %v", model, err)
	}
	q.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !q.WaitIdle(ctx) {
		t.Fatal("full queue blocked shutdown")
	}
}

func TestShutdownDoesNotWaitForQueuedModelLock(t *testing.T) {
	st := store.NewStore(t.TempDir())
	first, err := st.Create("first.ifc", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.Create("second.ifc", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &ctxAwareRunner{started: make(chan struct{})}
	q := testQueue(t, st, runner, 1)
	if !q.Enqueue(first.ID) {
		t.Fatal("first model not admitted")
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("first conversion did not start")
	}
	if !q.Enqueue(second.ID) {
		t.Fatal("second model not admitted")
	}
	db, err := artifact.Database(st.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = artifact.LockModel(context.Background(), tx, st.DataDir, second.ID); err != nil {
		t.Fatal(err)
	}
	q.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !q.WaitIdle(ctx) {
		t.Fatal("shutdown waited on a queued model's unrelated editing lock")
	}
}
