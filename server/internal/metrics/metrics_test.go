// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

package metrics

import (
	"encoding/json"
	"expvar"
	"testing"
)

func TestCounterOps(t *testing.T) {
	var c Counter
	c.Inc()
	c.Inc()
	c.Add(3)
	c.Dec()
	if got := c.Value(); got != 4 {
		t.Fatalf("Value = %d, want 4", got)
	}
	c.Set(9)
	if got := c.Value(); got != 9 {
		t.Fatalf("Set 后 Value = %d, want 9", got)
	}
}

// TestAibimVarPublished：/debug/vars 的 "aibim" 聚合变量存在且含全部姿态字段。
func TestAibimVarPublished(t *testing.T) {
	v := expvar.Get("aibim")
	if v == nil {
		t.Fatal(`expvar "aibim" 未发布`)
	}
	var m map[string]int64
	if err := json.Unmarshal([]byte(v.String()), &m); err != nil {
		t.Fatalf("aibim 不是 JSON map: %v", err)
	}
	for _, key := range []string{"goroutines", "sseSubscribers", "convertPending", "convertRunning", "editInFlight"} {
		if _, ok := m[key]; !ok {
			t.Errorf("aibim 缺字段 %q", key)
		}
	}
	SSESubscribers.Inc()
	defer SSESubscribers.Dec()
	if m2 := readAibim(t); m2["sseSubscribers"] != SSESubscribers.Value() {
		t.Errorf("sseSubscribers 未随 Inc 变化: %v", m2)
	}
}

func readAibim(t *testing.T) map[string]int64 {
	t.Helper()
	var m map[string]int64
	if err := json.Unmarshal([]byte(expvar.Get("aibim").String()), &m); err != nil {
		t.Fatal(err)
	}
	return m
}
