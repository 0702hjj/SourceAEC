// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func verifyLargeGeometryIterations(value string, clients int) error {
	if !strings.HasSuffix(value, "x") {
		return fmt.Errorf("large geometry requires a fixed -benchtime count")
	}
	count, err := strconv.Atoi(strings.TrimSuffix(value, "x"))
	if err != nil || count < clients {
		return fmt.Errorf("fixed -benchtime must cover at least %d clients", clients)
	}
	return nil
}

func TestLargeGeometryIterationsRequireCompleteBursts(t *testing.T) {
	for _, tc := range []struct {
		value   string
		clients int
		valid   bool
	}{
		{"4x", 4, true}, {"32x", 1, true}, {"1x", 4, false}, {"30s", 4, false}, {"badx", 1, false},
	} {
		if err := verifyLargeGeometryIterations(tc.value, tc.clients); (err == nil) != tc.valid {
			t.Fatalf("%s clients=%d: %v", tc.value, tc.clients, err)
		}
	}
}

type largeGeometryExecutor struct {
	sandbox         *Sandbox
	target          int
	peak, generated atomic.Int64
}

func (e *largeGeometryExecutor) Run(ctx context.Context, script, kind string) (Product, error) {
	p, err := e.sandbox.Run(ctx, script, kind)
	if err != nil {
		return p, err
	}
	if len(p.Data) < e.target*9/10 || len(p.Data) > e.target {
		return Product{}, fmt.Errorf("fixture requested %d bytes but generated %d", e.target, len(p.Data))
	}
	e.generated.Add(1)
	for previous := e.peak.Load(); int64(len(p.Data)) > previous; previous = e.peak.Load() {
		if e.peak.CompareAndSwap(previous, int64(len(p.Data))) {
			break
		}
	}
	return p, nil
}

type largeGeometryResults struct {
	mu        sync.Mutex
	counts    map[string]int
	latencies []time.Duration
	examples  map[string]string
}

func (r *largeGeometryResults) record(outcome string, elapsed time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.counts == nil {
		r.counts = map[string]int{}
		r.examples = map[string]string{}
	}
	r.counts[outcome]++
	if outcome == "ok" {
		r.latencies = append(r.latencies, elapsed)
	}
	if err != nil && r.examples[outcome] == "" {
		detail := err.Error()
		if len(detail) > 2048 {
			detail = detail[len(detail)-2048:]
		}
		r.examples[outcome] = detail
	}
}
func largeGeometryOutcome(err error) string {
	if err == nil {
		return "ok"
	}
	var domain *Error
	if !errors.As(err, &domain) {
		return "unexpected"
	}
	switch domain.Status {
	case 429:
		return "overload"
	case 504:
		return "timeout"
	case 422:
		if strings.Contains(domain.Detail, "exceeds inspection limit") || strings.Contains(domain.Detail, "exceeds scan limit") {
			return "inspection-limit"
		}
		for _, message := range []string{"MemoryError", "std::bad_alloc", "File too large"} {
			if strings.Contains(domain.Detail, message) {
				return "execution-limit"
			}
		}
	}
	return "unexpected"
}

func TestLargeGeometryOutcomeDoesNotHideInvalidFixtures(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, "ok"},
		{failure(429, "capacity"), "overload"},
		{failure(504, "deadline"), "timeout"},
		{failure(422, "DXF expansion exceeds inspection limit"), "inspection-limit"},
		{failure(422, "script execution failed: MemoryError"), "execution-limit"},
		{failure(422, "SyntaxError"), "unexpected"},
		{failure(422, "invalid DXF coordinates"), "unexpected"},
		{failure(503, "bubblewrap missing"), "unexpected"},
		{context.DeadlineExceeded, "unexpected"},
	} {
		if got := largeGeometryOutcome(tc.err); got != tc.want {
			t.Fatalf("%v: got %s, want %s", tc.err, got, tc.want)
		}
	}
}
