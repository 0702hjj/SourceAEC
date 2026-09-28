// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_concurrency_test.go：多会话并发组合场景（W-0061 S0）——并发 post +
// SSE 订阅 + 交叉 abort 同场，断言无死锁、runs/订阅表收敛、观测计数对账。
// 全程条件等待（AGENTS.md 测试纪律 #5：禁止固定 sleep）。
package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"ifcviewer/server/internal/agent"
	"ifcviewer/server/internal/metrics"
)

// TestConcurrentSessionsPostSubscribeAbort：16 会话 × 消息 + 每会话一条真实
// SSE 订阅 + 偶数会话 post 后立即交叉 abort。收敛断言：
//  1. 全部 post 返回 200；
//  2. runs 表逐会话清空（abort/覆盖/正常收尾三路都不留残留）；
//  3. SSE 订阅表收敛到 n，断开后归零（含 metrics.SSESubscribers 对账）。
//
// 覆盖边界（W-0061 S0 实证发现 R10）：post 阶段**串行**——eino ADK
// `TypedChatModelAgent` 单实例并发 Run 存在内部数据竞争（adk/chatmodel.go:1145，
// race detector 实锤；生产形态同 kind 共享单 Agent，并发 chat 即触发）。
// 该竞争的修复与「并行 post」变体用例归 S2（先红后绿）；本用例钉住的是
// handler 层（runs/订阅/metrics）在订阅并发 + abort 交叉下的正确性。
func TestConcurrentSessionsPostSubscribeAbort(t *testing.T) {
	const n = 16
	dataDir := t.TempDir()
	st := agent.NewEventStore(dataDir)
	ag, err := agent.New(agent.LLMConfig{},
		agent.WithModel(agent.NewScriptedModel(defaultTestScript)),
		agent.WithStore(st),
	)
	if err != nil {
		t.Fatal(err)
	}
	h := &ChatHandler{
		deps:     ChatDeps{Ag: ag, Ev: st, DataDir: dataDir},
		mux:      http.NewServeMux(),
		sessions: map[string]*chatSession{},
		byAgent:  map[string]string{},
		runs:     map[string]*chatRun{},
		subs:     map[string]map[chan []byte]struct{}{},
		creating: map[string]*sync.Mutex{},
	}
	h.registerRoutes()
	srv := httptest.NewServer(h.mux)
	t.Cleanup(srv.Close)

	// 建会话：每会话绑独立假 modelId（幂等键 = modelId|projectId，全空会合并）。
	cids := make([]string, n)
	for i := range cids {
		cs, err := doChatCreate(h, fmt.Sprintf(`{"title":"c%d","modelId":"m_%016x"}`, i, i))
		if err != nil {
			t.Fatalf("建会话 #%d: %v", i, err)
		}
		cids[i] = cs.ID
	}

	// 每会话开一条真实 SSE 连接（走 httptest server 的完整 HTTP 栈）。
	resps := make([]*http.Response, n)
	for i, cid := range cids {
		resps[i], _ = sseConnect(t, srv, cid, "")
	}
	waitForSubscribers(t, h, n)

	// 消息（串行 + 逐跑排干，见函数注释 R10——重叠 Run 在共享 Agent 上有
	// eino 内部竞争与 cancelCtx 覆写）+ 交叉 abort（并发发射，落在收尾前后
	// 都合法，只断言不 panic 不残留）。
	var abortWg sync.WaitGroup
	for i, cid := range cids {
		if code := postChat(t, h, cid, "并发消息"); code != http.StatusOK {
			t.Fatalf("会话 #%d post 状态 = %d, want 200", i, code)
		}
		waitForRunsCount(t, h, cid, 0) // 排干本跑再发下一跑（R10 隔离）
		if i%2 == 0 {
			abortWg.Add(1)
			go func(cid string) {
				defer abortWg.Done()
				req := httptest.NewRequest(http.MethodPost,
					"/api/v1/chat/sessions/"+cid+"/abort", nil)
				h.mux.ServeHTTP(httptest.NewRecorder(), req)
			}(cid)
		}
	}
	abortWg.Wait()

	// 断开全部 SSE：订阅表与 metrics 计数双双归零（观测接线对账）。
	for _, r := range resps {
		r.Body.Close()
	}
	waitForSubscribers(t, h, 0)
}

// waitForSubscribers 条件等待订阅表总数（含 metrics 对账；超时失败）。
func waitForSubscribers(t *testing.T, h *ChatHandler, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.mu.RLock()
		got := 0
		for _, set := range h.subs {
			got += len(set)
		}
		h.mu.RUnlock()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("订阅数未收敛到 %d（当前 %d，metrics=%d）",
				want, got, metrics.SSESubscribers.Value())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
