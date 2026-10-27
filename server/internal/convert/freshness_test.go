// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
)

func TestEnqueueIfStaleUsesCommittedIdentity(t *testing.T) {
	st := store.NewStore(t.TempDir())
	m, err := st.Create("synthetic.ifc", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &countingRunner{}
	q := testQueue(t, st, runner, 1)
	q.Start(context.Background())
	if !q.EnqueueIfStale(m.ID) {
		t.Fatal("missing preview was not queued")
	}
	waitStatus(t, st, m.ID, "ready")
	if q.EnqueueIfStale(m.ID) {
		t.Fatal("unchanged source queued again")
	}
	// A touched projection does not invalidate an otherwise identical source.
	future := time.Now().Add(time.Hour)
	if err = os.Chtimes(st.IFCPath(m.ID), future, future); err != nil {
		t.Fatal(err)
	}
	if q.EnqueueIfStale(m.ID) {
		t.Fatal("mtime was treated as source identity")
	}
	if err = artifact.WriteFile(st.IFCPath(m.ID), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if !q.EnqueueIfStale(m.ID) {
		t.Fatal("changed source was not queued")
	}
	waitStatus(t, st, m.ID, "ready")
	if runner.count() != 2 {
		t.Fatalf("runs=%d, want 2", runner.count())
	}
	preview, err := st.ViewerPath(context.Background(), m.ID, "metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(preview); err != nil {
		t.Fatal(err)
	}
	if !q.EnqueueIfStale(m.ID) {
		t.Fatal("incomplete viewer pair was not queued")
	}
	waitStatus(t, st, m.ID, "ready")
	if runner.count() != 3 {
		t.Fatalf("runs=%d, want 3", runner.count())
	}
}
