// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadConfigConcurrencyDefaults：W-0061 新增并发/超时字段的默认值与 env 覆盖。
func TestLoadConfigConcurrencyDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server_config.json")
	if err := os.WriteFile(path, []byte(`{"dataDir": "d", "pprofAddr": ""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PprofAddr != "127.0.0.1:6060" {
		t.Errorf("pprofAddr 默认 = %q, want 127.0.0.1:6060", cfg.PprofAddr)
	}
	if cfg.LLMTimeoutS != 120 {
		t.Errorf("llmTimeoutS 默认 = %d, want 120", cfg.LLMTimeoutS)
	}
	if cfg.ShutdownJoinS != 10 {
		t.Errorf("shutdownJoinS 默认 = %d, want 10", cfg.ShutdownJoinS)
	}

	t.Setenv("VIEWER_PPROF_ADDR", "disable")
	t.Setenv("VIEWER_LLM_TIMEOUT_S", "77")
	t.Setenv("VIEWER_SSE_HEARTBEAT_S", "5")
	t.Setenv("VIEWER_SHUTDOWN_JOIN_S", "3")
	cfg, err = loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PprofAddr != "" {
		t.Errorf("pprofAddr disable 后 = %q, want 空", cfg.PprofAddr)
	}
	if cfg.LLMTimeoutS != 77 || cfg.SSEHeartbeatS != 5 || cfg.ShutdownJoinS != 3 {
		t.Errorf("env 覆盖失效: %d/%d/%d", cfg.LLMTimeoutS, cfg.SSEHeartbeatS, cfg.ShutdownJoinS)
	}
}

// TestNewHTTPServerTimeoutPolicy：S1 超时纪律——头部/空闲超时必须设置；
// **WriteTimeout/ReadTimeout 必须为零**（WriteTimeout 掐死 SSE、ReadTimeout
// 误杀慢速大上传——红线钉死）。
func TestNewHTTPServerTimeoutPolicy(t *testing.T) {
	srv := newHTTPServer("127.0.0.1:0", http.NotFoundHandler())
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 10s", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v, want 120s", srv.IdleTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0（SSE 红线）", srv.WriteTimeout)
	}
	if srv.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %v, want 0（大上传红线）", srv.ReadTimeout)
	}
}
