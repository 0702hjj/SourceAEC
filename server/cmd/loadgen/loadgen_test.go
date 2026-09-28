// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPercentiles(t *testing.T) {
	// 1..100 排序后：P50=50（index 49 向上取 50）、P99≈99、P99.9≈100。
	lat := make([]float64, 0, 100)
	for i := 1; i <= 100; i++ {
		lat = append(lat, float64(i))
	}
	p50, p99, p999 := percentiles(sortLat(lat))
	if p50 != 50 {
		t.Errorf("P50 = %v, want 50", p50)
	}
	if p99 != 99 {
		t.Errorf("P99 = %v, want 99", p99)
	}
	if p999 != 100 {
		t.Errorf("P99.9 = %v, want 100", p999)
	}
	if _, _, _ = percentiles(nil); true {
		// 空切片不得 panic
		if p50, _, _ = percentiles(nil); p50 != 0 {
			t.Errorf("空切片 P50 = %v, want 0", p50)
		}
	}
}

func TestParseModelsEnvelope(t *testing.T) {
	ids, err := parseModelsEnvelope([]byte(
		`{"code":0,"message":"ok","data":[{"id":"m_a"},{"id":"m_b"}]}`))
	if err != nil || len(ids) != 2 || ids[0] != "m_a" || ids[1] != "m_b" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	if _, err := parseModelsEnvelope([]byte(`not-json`)); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
}

func TestParseModelID(t *testing.T) {
	id, err := parseModelID([]byte(`{"code":0,"message":"ok","data":{"id":"m_x"}}`))
	if err != nil || id != "m_x" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if _, err := parseModelID([]byte(`{"code":0,"data":{}}`)); err == nil {
		t.Fatal("无 data.id 应报错")
	}
}

// TestHoldConnsAgainstFakeStream：httptest 假流（开连接即写注释帧并挂住），
// holdConns 应全部建立、零断裂——工具核心路径的冒烟。
func TestHoldConnsAgainstFakeStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	urls := make([]string, 8)
	for i := range urls {
		urls[i] = srv.URL
	}
	est, drop := holdConns(newCommon(srv.URL, "", ""), urls, 500*time.Millisecond)
	if est != 8 {
		t.Errorf("established = %d, want 8", est)
	}
	if drop != 0 {
		t.Errorf("dropped = %d, want 0", drop)
	}
}

// TestHoldConnsAgainstSilentStream：连响应头都不发的静默 handler——客户端
// 永久阻塞在 Do，唯一可靠收尾是请求级 ctx 取消（S0 实证过两代坑：deadline
// 检查不生效、事后 Close body 解不开未返回的 Do）。断言：工具不挂死（回归
// 钉：修复前挂到 go test 超时）+ 计时收尾不计断裂（drop=0）；established
// 允许为 0——头没上线本就不算建立。
func TestHoldConnsAgainstSilentStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	urls := []string{srv.URL, srv.URL}
	done := make(chan struct{})
	var est, drop int64
	go func() {
		defer close(done)
		est, drop = holdConns(newCommon(srv.URL, "", ""), urls, 300*time.Millisecond)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("holdConns 对静默流挂死——deadline 收尾失效（回归）")
	}
	if drop != 0 {
		t.Errorf("dropped = %d, want 0（计时收尾不得计为断裂）", drop)
	}
	if est > 2 {
		t.Errorf("established = %d, want ≤ 2", est)
	}
}

// TestSampleAgainstObsServer：观测采样对假 /debug/vars 的解析。
func TestSampleAgainstObsServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"aibim":    map[string]any{"goroutines": 42},
			"memstats": map[string]any{"Alloc": 10e6, "Sys": 64e6},
		})
	}))
	defer srv.Close()
	c := newCommon("http://127.0.0.1:1", srv.URL, "")
	s, err := c.sample()
	if err != nil {
		t.Fatal(err)
	}
	if s.Goroutine != 42 || s.AllocMB != 10 || s.SysMB != 64 {
		t.Fatalf("sample = %+v", s)
	}
}
