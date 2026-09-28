// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_shutdown_test.go：W-0061 S1 停机语义——SSE 心跳按拍、根 ctx 取消
// 收尾在途 run、DrainBackground join 后台 goroutine。条件等待纪律。
package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"ifcviewer/server/internal/agent"
)

// TestSSEHeartbeatBeats：注入 50ms 心跳，空闲流上必须在 2s 内收到
// ": keepalive" 注释帧（EventSource 忽略、反代 idle timeout 不掐）。
func TestSSEHeartbeatBeats(t *testing.T) {
	h, srv, cid := newSSETestHandler(t)
	h.deps.SSEHeartbeat = 50 * time.Millisecond // 注入短拍（构造后注入：handler 未共享）
	resp, reader := sseConnect(t, srv, cid, "")
	defer resp.Body.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("SSE read: %v", err)
		}
		if strings.HasPrefix(strings.TrimSpace(line), ": keepalive") {
			return
		}
	}
	t.Fatal("2s 内未收到 keepalive 心跳帧")
}

// ctxBlockModel：尊重 ctx 的挂起模型——Stream 阻塞到 ctx 取消才返回错误
// （模拟带超时/取消的真实 LLM 客户端；countingModel 不看 ctx，测不了停机路径）。
type ctxBlockModel struct{}

func (m *ctxBlockModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *ctxBlockModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *ctxBlockModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestRootCtxCancelEndsRunsAndDrains：根 ctx（server 生命期）取消 → 在途
// run 收尾（runs 表清空）→ DrainBackground 在预算内返回 true——停机全路径
// join 的行为锚点。
func TestRootCtxCancelEndsRunsAndDrains(t *testing.T) {
	dataDir := t.TempDir()
	st := agent.NewEventStore(dataDir)
	ag, err := agent.New(agent.LLMConfig{},
		agent.WithModel(&ctxBlockModel{}), agent.WithStore(st))
	if err != nil {
		t.Fatal(err)
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	h := &ChatHandler{
		deps:     ChatDeps{Ag: ag, Ev: st, DataDir: dataDir, RootCtx: rootCtx},
		mux:      http.NewServeMux(),
		sessions: map[string]*chatSession{},
		byAgent:  map[string]string{},
		runs:     map[string]*chatRun{},
		subs:     map[string]map[chan []byte]struct{}{},
		creating: map[string]*sync.Mutex{},
	}
	h.registerRoutes()
	cs, err := doChatCreate(h, `{"title":"t"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := postChat(t, h, cs.ID, "挂起"); code != http.StatusOK {
		t.Fatalf("post status = %d", code)
	}
	waitForRunsCount(t, h, cs.ID, 1)

	rootCancel() // 停机：根取消
	waitForRunsCount(t, h, cs.ID, 0)

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !h.DrainBackground(drainCtx) {
		t.Fatal("DrainBackground 未在预算内收尾（后台 goroutine 滞留）")
	}
}

// TestDrainBackgroundWaitsForSlowTask：goBg 任务慢速收尾时 DrainBackground
// 等待而非立即返回（join 语义，非 fire-and-forget）。
func TestDrainBackgroundWaitsForSlowTask(t *testing.T) {
	h := &ChatHandler{}
	release := make(chan struct{})
	h.goBg(func() { <-release })
	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- h.DrainBackground(drainCtx) }()
	select {
	case <-done:
		t.Fatal("DrainBackground 未等待在途任务就返回")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("任务完成后 DrainBackground 应返回 true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("任务完成后 DrainBackground 未返回")
	}
}

// TestSSEMissedReplayUnaffectedByHeartbeat：心跳不进入重同步缓冲——补发
// 语义与既有 Last-Event-ID 行为不因心跳改变（回归钉）。
func TestSSEMissedReplayUnaffectedByHeartbeat(t *testing.T) {
	h, srv, cid := newSSETestHandler(t)
	h.deps.SSEHeartbeat = 5 * time.Second // 长拍：本用例不依赖心跳
	resp, reader := sseConnect(t, srv, cid, "")
	defer resp.Body.Close()
	h.pushSystem(cid, "first.event", map[string]any{"n": 1}) // 首发：给重放缓冲一个基准 id
	frames := readSSEFrames(t, reader, 1)
	firstID := frames[0].id
	resp.Body.Close()

	resp2, reader2 := sseConnect(t, srv, cid, firstID)
	defer resp2.Body.Close()
	// 上一连接读到 id=firstID；再推一帧，Last-Event-ID 之后应只补这一帧、
	// 无心跳污染（keepalive 是注释行，不入缓冲）。
	h.pushSystem(cid, "probe.event", map[string]any{"ok": true})
	got := readSSEFrames(t, reader2, 1)
	if got[0].event != "probe.event" {
		t.Fatalf("补发后新帧 event = %q, want probe.event", got[0].event)
	}
}
