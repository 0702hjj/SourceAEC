// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
)

type revisionRunner struct {
	started chan string
	release chan struct{}
	read    chan string
}

func (r *revisionRunner) Run(ctx context.Context, in, out string) error {
	r.started <- in
	select {
	case <-r.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	r.read <- string(data)
	if err = os.WriteFile(filepath.Join(out, "model.xkt"), data, 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "metadata.json"), []byte(`{}`), 0600)
}

func TestConversionUsesSnapshotAndRejectsStaleCompletion(t *testing.T) {
	st := store.NewStore(t.TempDir())
	if err := artifact.Open(st.DataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifact.Close(st.DataDir) })
	m, err := st.Create("synthetic.ifc", 3, strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	r := &revisionRunner{make(chan string, 2), make(chan struct{}), make(chan string, 2)}
	q := testQueue(t, st, r, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	q.Start(ctx)
	t.Cleanup(func() {
		cancel()
		deadline, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if !q.WaitIdle(deadline) {
			t.Error("queue did not stop")
		}
	})
	q.Enqueue(m.ID)
	select {
	case <-r.started:
	case <-ctx.Done():
		t.Fatal("first conversion did not start")
	}
	if err = artifact.WriteFile(st.IFCPath(m.ID), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	q.Enqueue(m.ID)
	r.release <- struct{}{}
	select {
	case got := <-r.read:
		if got != "old" {
			t.Errorf("converter read mutable source: %q", got)
		}
	case <-ctx.Done():
		t.Fatal("first conversion did not complete")
	}
	select {
	case <-r.started:
	case <-ctx.Done():
		t.Fatal("replacement conversion did not start")
	}
	current, err := st.Get(m.ID)
	if err != nil || current.Status != "converting" {
		t.Fatalf("old completion marked new source ready: %+v %v", current, err)
	}
	r.release <- struct{}{}
	waitStatus(t, st, m.ID, "ready")
}
