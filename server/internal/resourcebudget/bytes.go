// SPDX-License-Identifier: Apache-2.0
// Package resourcebudget provides process-local byte reservations. Reservations
// describe retained payloads, not a promise about total process RSS.
package resourcebudget

import "sync"

type Bytes struct {
	mu          sync.Mutex
	limit, used int64
}

func NewBytes(limit int64) *Bytes { return &Bytes{limit: limit} }

// Try reserves before allocating a payload. The returned release is idempotent.
func (b *Bytes) Try(n int64) (func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n < 0 || n > b.limit-b.used {
		return nil, false
	}
	b.used += n
	var once sync.Once
	return func() { once.Do(func() { b.mu.Lock(); b.used -= n; b.mu.Unlock() }) }, true
}
