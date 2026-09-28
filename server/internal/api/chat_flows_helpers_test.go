// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_flows_helpers_test.go：W-0051 全链 SSE 帧序列回归的共享夹具与断言 helper
// （catch-all 假 edit-service、kind 主 agent 装配镜像、SSE 帧解析/序列断言）。
// 用例见 chat_flows_contract_test.go（plan→cad→ifc / cad 带计划）与
// chat_flows_ifc_failure_test.go（ifc 独立 / 失败路径）。
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"

	"ifcviewer/server/internal/agent"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/editsvc"
	"ifcviewer/server/internal/store"
)

// --- 测试夹具：catch-all 假 edit-service + kind 主 agent 装配镜像 ---

// flowEditFake 是 catch-all 假 edit-service：未预置路由一律 200 {"ok":true}——
// 骨架 init 的 modelId 运行期随机、路由不可预知（与 fakePy2 的「未预置 404」
// 语义互补：这里要的是全链成功路径）。calls 记录做调用序断言。
type flowEditFake struct {
	mu     sync.Mutex
	calls  []string
	routes map[string]string // "METHOD /path" -> 200 body
	srv    *httptest.Server
}

func newFlowEditFake(t *testing.T) *flowEditFake {
	t.Helper()
	f := &flowEditFake{routes: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		resp, ok := f.routes[r.Method+" "+r.URL.Path]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			resp = `{"ok":true}`
		}
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *flowEditFake) set(method, path, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = body
}

// countContaining 统计包含 substr 的调用次数（读副本，notify 异步期间也安全）。
func (f *flowEditFake) countContaining(substr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			n++
		}
	}
	return n
}

// flowHarness 汇集三类对话共用装配产物（handler + 项目/方案存储 + 双假后端 + 转换队列）。
type flowHarness struct {
	h      *ChatHandler
	ps     *store.ProjectStore
	planSt *store.PlanStore
	ifcEd  *flowEditFake
	cadEd  *flowEditFake
	runs   chan string
}

// newFlowHarness 构造 chat handler 底座（双假后端 + 项目/方案存储 + 转换队列 +
// fake aiplan）——不含 agent：项目创建（REST）不依赖 agent，先建项目拿到随机
// modelId 后再装配 scripted kind agent（脚本参数内插 modelId 的确定性前提）。
func newFlowHarness(t *testing.T) *flowHarness {
	t.Helper()
	dataDir := t.TempDir()
	st := store.NewStore(dataDir)
	ps := store.NewProjectStore(dataDir)
	planSt := store.NewPlanStore(dataDir)
	runs := make(chan string, 16)
	q := convert.NewQueue(st, okRunner2{runs: runs}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	q.Start(ctx)
	binDir := t.TempDir()
	if err := writeFakeAiplan(t, filepath.Join(binDir, "aiplan")); err != nil {
		t.Fatal(err)
	}
	ifcEd, cadEd := newFlowEditFake(t), newFlowEditFake(t)
	evStore := agent.NewEventStore(dataDir)
	h := &ChatHandler{
		deps: ChatDeps{
			St: st, Ps: ps, PlanSt: planSt,
			Ed: editsvc.New(ifcEd.srv.URL), Cad: editsvc.New(cadEd.srv.URL),
			Q: q, DataDir: dataDir, Ev: evStore, AiplanBin: filepath.Join(binDir, "aiplan"),
		},
		mux:      http.NewServeMux(),
		sessions: map[string]*chatSession{},
		byAgent:  map[string]string{},
		runs:     map[string]*chatRun{},
		subs:     map[string]map[chan []byte]struct{}{},
		creating: map[string]*sync.Mutex{},
	}
	h.registerRoutes()
	return &flowHarness{h: h, ps: ps, planSt: planSt, ifcEd: ifcEd, cadEd: cadEd, runs: runs}
}

// attachKindAgent 装配 kind 主 agent（WithKind，main.go kind agent 装配的测试
// 镜像）。childIfc/childCad 是子 agent 的 scripted 脚本：agent.New 内 ifc 角色
// 先建、cad 角色后建（agent.go 装配顺序固定），childModel 工厂按调用序取脚本。
// childCad 传 nil 表示该 kind 子代理不可达/不用（仍占工厂调用序位）。
func (fw *flowHarness) attachKindAgent(t *testing.T, kind string, main agent.Script, childIfc, childCad *agent.Script) {
	t.Helper()
	scripts := []*agent.Script{childIfc, childCad}
	idx := 0
	childFactory := func() model.ToolCallingChatModel {
		s := scripts[idx%len(scripts)]
		idx++
		if s == nil {
			return agent.NewScriptedModel(agent.Script{})
		}
		return agent.NewScriptedModel(*s)
	}
	ag, err := agent.New(agent.LLMConfig{},
		agent.WithModel(agent.NewScriptedModel(main)),
		agent.WithStore(fw.h.deps.Ev),
		agent.WithTools(fw.h.DomainTools()),
		agent.WithChildModelFactory(childFactory),
		agent.WithDataDir(fw.h.deps.DataDir),
		agent.WithKind(kind),
	)
	if err != nil {
		t.Fatalf("agent.New(kind=%s): %v", kind, err)
	}
	fw.h.SetAgent(ag)
	fw.h.SetAgents(map[string]*agent.Agent{kind: ag})
}

// --- SSE 帧解析 / 断言 helper ---

// flowReq 走 mux 发一条 REST 请求并断言 200。
func flowReq(t *testing.T, h *ChatHandler, method, path, body string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s status = %d body = %s", method, path, rec.Code, rec.Body)
	}
}

// flowFrameData 解一条 SSE 帧的 data JSON（pushSystem 帧：id/event/data 三行）。
func flowFrameData(t *testing.T, frame string) map[string]any {
	t.Helper()
	idx := strings.Index(frame, "data: ")
	if idx < 0 {
		t.Fatalf("帧缺 data 行: %q", frame)
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(frame[idx+len("data: "):])), &d); err != nil {
		t.Fatalf("data 非 JSON: %v（帧 %q）", err, frame)
	}
	return d
}

func flowStr(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// flowEventNames 帧序列 → 事件名序列。
func flowEventNames(t *testing.T, frames []string) []string {
	t.Helper()
	names := make([]string, 0, len(frames))
	for _, f := range frames {
		names = append(names, frameEvent(f))
	}
	return names
}

// assertInject 钉一条系统注入帧：恰好一条、载荷字段符合、且先于锚帧（锚 =
// 对应工具卡的 completed 帧——直推必先于其 completed 的确定性不变量）。
func assertInject(t *testing.T, frames []string, event string, want map[string]any, anchorIdx int) {
	t.Helper()
	idx := flowIndexOf(frames, event, "")
	if idx < 0 {
		t.Fatalf("未收到 %s 帧:\n%s", event, strings.Join(frames, "---\n"))
	}
	if flowIndexOf(frames[idx+1:], event, "") >= 0 {
		t.Fatalf("%s 帧应恰好一条", event)
	}
	d := flowFrameData(t, frames[idx])
	for k, v := range want {
		if d[k] != v {
			t.Fatalf("%s 载荷字段 %s = %v, want %v（全部: %v）", event, k, d[k], v, d)
		}
	}
	if anchorIdx >= 0 && idx > anchorIdx {
		t.Fatalf("%s（下标 %d）应先于锚帧（下标 %d）", event, idx, anchorIdx)
	}
}

// flowInjectEvents 是不经翻译层、由工具执行内直接 pushSystem 的系统注入事件
// （viewer.staged/model.created 等）：与翻译帧的消费路径不同（直推 vs 事件通道
// 异步翻译），与相邻工具卡的交错位置受调度影响——契约钉「存在 + 载荷 + 先于
// 对应工具卡的 completed 帧」（确定性不变量），不钉严格次序。
var flowInjectEvents = map[string]bool{
	"viewer.staged": true, "model.created": true,
	"viewer.committed": true, "viewer.notify_failed": true,
}

// assertEventSeq 钉翻译帧序列（事件名逐条相等；系统注入帧过滤后比对——它们的
// 独立断言见各用例。失配时打印收到的帧全文供比对）。
func assertEventSeq(t *testing.T, frames []string, want []string) {
	t.Helper()
	var got []string
	for _, f := range frames {
		if e := frameEvent(f); !flowInjectEvents[e] {
			got = append(got, e)
		}
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("SSE 事件序列不符：\n got: %v\nwant: %v\n帧流：\n%s", got, want, strings.Join(frames, "---\n"))
	}
}

// flowSub 是一条 subagent.status 帧的提取值。
type flowSub struct{ id, status, persona, task string }

// flowSubStatuses 按出现序提取全部 subagent.status 帧。
func flowSubStatuses(t *testing.T, frames []string) []flowSub {
	t.Helper()
	var out []flowSub
	for _, f := range frames {
		if frameEvent(f) != "subagent.status" {
			continue
		}
		d := flowFrameData(t, f)
		out = append(out, flowSub{
			id: flowStr(d, "subagentId"), status: flowStr(d, "status"),
			persona: flowStr(d, "persona"), task: flowStr(d, "task"),
		})
	}
	return out
}

// flowCard 是一张工具卡片 part 帧的提取值（subID 空 = 主会话卡）。
type flowCard struct{ tool, status, output, errText, subID string }

// flowToolCards 按出现序提取全部工具卡 part 帧（含 running / completed / error 态）。
func flowToolCards(t *testing.T, frames []string) []flowCard {
	t.Helper()
	var out []flowCard
	for _, f := range frames {
		if frameEvent(f) != "message.part.updated" {
			continue
		}
		d := flowFrameData(t, f)
		part, _ := d["part"].(map[string]any)
		if part == nil || flowStr(part, "type") != "tool" {
			continue
		}
		c := flowCard{tool: flowStr(part, "tool"), subID: flowStr(d, "subagentId")}
		if st, _ := part["state"].(map[string]any); st != nil {
			c.status = flowStr(st, "status")
			c.output = flowStr(st, "output")
			c.errText = flowStr(st, "error")
		}
		out = append(out, c)
	}
	return out
}

// assertCompletedCards 钉「已完成工具卡」的 (tool, subagentId) 序列。
func assertCompletedCards(t *testing.T, frames []string, want []flowCard) {
	t.Helper()
	var got []flowCard
	for _, c := range flowToolCards(t, frames) {
		if c.status == "completed" {
			got = append(got, c)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("completed 卡数 = %d, want %d:\n%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].tool != want[i].tool || got[i].subID != want[i].subID {
			t.Fatalf("completed 卡[%d] = (%s,%s), want (%s,%s)", i, got[i].tool, got[i].subID, want[i].tool, want[i].subID)
		}
	}
}

// flowIndexOf 首个「事件名 + 帧含 substr」帧的下标（找不到 -1）。
func flowIndexOf(frames []string, event, substr string) int {
	for i, f := range frames {
		if frameEvent(f) == event && (substr == "" || strings.Contains(f, substr)) {
			return i
		}
	}
	return -1
}

// flowAnswer 回答审批中断（POST /answer）并收完本轮剩余帧（到 session.idle）。
func flowAnswer(t *testing.T, h *ChatHandler, ch chan []byte, cid string, postFrames []string, answer string) []string {
	t.Helper()
	interruptID := ""
	for _, f := range postFrames {
		if frameEvent(f) == "question.ask" {
			interruptID = frameDataStr(t, f, "interruptId")
		}
	}
	if interruptID == "" {
		t.Fatalf("post 阶段未收到 question.ask:\n%s", strings.Join(postFrames, "---\n"))
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/sessions/"+cid+"/answer",
		strings.NewReader(`{"interruptId":"`+interruptID+`","answer":"`+answer+`"}`))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("answer status = %d body = %s", rec.Code, rec.Body)
	}
	return collectUntil(t, ch, "session.idle")
}
