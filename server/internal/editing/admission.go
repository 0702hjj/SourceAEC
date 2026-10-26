// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"sync"
)

// Bound hot-model queues without occupying PostgreSQL connections. This gate
// is process-local; the database advisory lock remains the cross-process guard.
const modelWaitLimit = 64

type modelSlot struct {
	permit chan struct{}
	refs   int
}

type modelAdmission struct {
	mu    sync.Mutex
	slots map[string]*modelSlot
}

var editAdmission = &modelAdmission{slots: make(map[string]*modelSlot)}

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
	if slot.refs >= modelWaitLimit+1 {
		a.mu.Unlock()
		return nil, failure(429, "model request queue limit reached")
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
