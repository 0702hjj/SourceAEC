// SPDX-License-Identifier: Apache-2.0
package resourcebudget

import (
	"sync"
	"testing"
)

func TestBytesConcurrentReservationsAndRelease(t *testing.T) {
	b := NewBytes(10)
	release, ok := b.Try(6)
	if !ok {
		t.Fatal("initial reservation")
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, ok := b.Try(5); ok {
				t.Error("overcommitted budget")
			}
		})
	}
	wg.Wait()
	for range 20 {
		wg.Go(release)
	}
	wg.Wait()
	all, ok := b.Try(10)
	if !ok {
		t.Fatal("reservation was not released exactly once")
	}
	if _, ok := b.Try(1); ok {
		t.Fatal("accepted exhausted budget")
	}
	all()
	if _, ok := b.Try(-1); ok {
		t.Fatal("accepted negative bytes")
	}
}
