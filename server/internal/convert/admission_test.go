// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
)

func TestConversionSurvivesFullModelAdmission(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprintf("shutdown=%v", shutdown), func(t *testing.T) {
			testFullModelAdmission(t, shutdown)
		})
	}
}

func testFullModelAdmission(t *testing.T, shutdown bool) {
	st := store.NewStore(t.TempDir())
	m, err := st.Create("synthetic.ifc", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &countingRunner{}
	q := testQueue(t, st, runner, 1)
	release, err := artifact.AcquireModel(context.Background(), st.DataDir, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q.Start(ctx)
	var wg sync.WaitGroup
	for i := 0; i < artifact.ModelWaitLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := artifact.AcquireModel(ctx, st.DataDir, m.ID)
			if err == nil {
				unlock()
			}
		}()
	}
	t.Cleanup(func() { cancel(); wg.Wait() })
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		// The exported waiting gauge increments before namespace resolution
		// and registry admission. It cannot synchronize a full registry. Probe
		// actual overload instead; a not-yet-full probe has its own deadline.
		probe, stop := context.WithTimeout(ctx, 20*time.Millisecond)
		unlock, err := artifact.AcquireModel(probe, st.DataDir, m.ID)
		stop()
		if err == nil {
			unlock()
			t.Fatal("probe acquired a held model")
		}
		if errors.Is(err, artifact.ErrModelQueueFull) {
			break
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("model gate did not fill")
		}
	}
	if !q.Enqueue(m.ID) {
		t.Fatal("model overload dropped conversion before bounded queue admission")
	}
	for runner.count() == 0 {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("conversion did not execute")
		}
	}
	if shutdown {
		q.Close()
		stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if !q.WaitIdle(stopCtx) {
			t.Fatal("full model admission prevented conversion shutdown")
		}
		return
	}
	// Cancel all queued writers, then permit publication. The converter must
	// retain its result through overload without running geometry a second time.
	cancel()
	wg.Wait()
	release()
	waitStatus(t, st, m.ID, "ready")
	if runner.count() != 1 {
		t.Fatalf("geometry repeated during publication admission: %d", runner.count())
	}
}
