// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"errors"
	"sync"

	"ifcviewer/server/internal/metrics"
)

// Bound hot-model queues without occupying PostgreSQL connections. This gate
// is process-local; the database advisory lock remains the cross-process guard.
const ModelWaitLimit = 64

type modelSlot struct {
	permit chan struct{}
	refs   int
}

type modelAdmission struct {
	mu    sync.Mutex
	slots map[string]*modelSlot
}

var processModels = &modelAdmission{slots: make(map[string]*modelSlot)}

var ErrModelQueueFull = errors.New("model request queue limit reached")

// AcquireModel admits a model writer before it obtains a database connection.
// All callers must release after their transaction and projection recovery.
// PostgreSQL still provides the cross-process lock; this budget is process-local.
func AcquireModel(ctx context.Context, root, model string) (func(), error) {
	metrics.ModelWaiting.Inc()
	defer metrics.ModelWaiting.Dec()
	namespace, err := Namespace(root)
	if err != nil {
		return nil, err
	}
	release, err := processModels.acquire(ctx, namespace+":"+model)
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(release) }, nil
}

func (a *modelAdmission) acquire(ctx context.Context, key string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	slot := a.slots[key]
	if slot == nil {
		slot = &modelSlot{permit: make(chan struct{}, 1)}
		slot.permit <- struct{}{}
		a.slots[key] = slot
	}
	if slot.refs >= ModelWaitLimit+1 {
		a.mu.Unlock()
		return nil, ErrModelQueueFull
	}
	slot.refs++
	a.mu.Unlock()
	forget := func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		slot.refs--
		if slot.refs == 0 {
			delete(a.slots, key)
		}
	}
	select {
	case <-ctx.Done():
		forget()
		return nil, ctx.Err()
	case <-slot.permit:
		release := func() {
			slot.permit <- struct{}{}
			forget()
		}
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		return release, nil
	}
}
