// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestModelAdmissionBoundsQueueAndRecoversAfterCancellation(t *testing.T) {
	a := &modelAdmission{slots: make(map[string]*modelSlot)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release, err := a.acquire(ctx, "hot")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	waitCtx, stop := context.WithCancel(ctx)
	defer stop()
	var workers sync.WaitGroup
	for i := 0; i < ModelWaitLimit; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			unlock, err := a.acquire(waitCtx, "hot")
			if err == nil {
				unlock()
				t.Error("waiter acquired held model")
			} else if !errors.Is(err, context.Canceled) {
				t.Errorf("unexpected waiter error: %v", err)
			}
		}()
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		a.mu.Lock()
		refs := a.slots["hot"].refs
		a.mu.Unlock()
		if refs == ModelWaitLimit+1 {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("waiters did not reach admission")
		}
	}
	_, err = a.acquire(ctx, "hot")
	if !errors.Is(err, ErrModelQueueFull) {
		t.Fatalf("expected bounded overload, got %v", err)
	}
	// Another model must still be admitted while the hot model is full.
	other, err := a.acquire(ctx, "other")
	if err != nil {
		t.Fatal(err)
	}
	other()
	stop()
	workers.Wait()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.slots) != 1 || a.slots["hot"].refs != 1 {
		t.Fatalf("canceled waiters retained slots: %v", a.slots)
	}
}

func TestSharedModelAdmissionReleaseAndNamespace(t *testing.T) {
	t.Setenv("VIEWER_STORAGE_NAMESPACE", "")
	first, second := t.TempDir(), t.TempDir()
	release, err := AcquireModel(context.Background(), first, "same-model")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	other, err := AcquireModel(ctx, second, "same-model")
	if err != nil {
		t.Fatal("different namespace blocked", err)
	}
	other()
	other() // Public release must tolerate cleanup after early cancellation.
	release()
	release()
	recovered, err := AcquireModel(ctx, first, "same-model")
	if err != nil {
		t.Fatal("model permit did not recover", err)
	}
	recovered()
}

func TestModelAdmissionForgetsIdleModels(t *testing.T) {
	a := &modelAdmission{slots: make(map[string]*modelSlot)}
	for i := 0; i < 10; i++ {
		release, err := a.acquire(context.Background(), "model")
		if err != nil {
			t.Fatal(err)
		}
		release()
		if len(a.slots) != 0 {
			t.Fatal("idle model retained")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.acquire(ctx, "canceled"); !errors.Is(err, context.Canceled) || len(a.slots) != 0 {
		t.Fatalf("pre-canceled request changed admission: %v", err)
	}
}
