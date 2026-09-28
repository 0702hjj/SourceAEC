// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// agent_run.go：Agent 运行域（从 agent.go 拆出，W-0057 行数门控）——
// Run（一轮 ReAct + 事件扇出 + 会话记忆回填）与 Resume（HITL 续跑），
// 及 in-memory CheckPointStore（中断状态存取）。
package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// turnCount 返回历史父 turn/start 数（Run/Resume 共用基准）：
// 子 agent 事件（含其 turn/start）不打扰父 turn 计数。store nil（离线/测试）返回 0。
// 语义差异：Run 新开一轮 → turn = count+1；Resume 继续当前轮 → turn = max(count, 1)。
func (a *Agent) turnCount(sessionID string) (int, error) {
	if a.store == nil {
		return 0, nil
	}
	prev, err := a.store.Load(sessionID)
	if err != nil {
		return 0, fmt.Errorf("load session %s: %w", sessionID, err)
	}
	n := 0
	for _, ev := range prev {
		if ev.Type == EventTurnStart && ev.SubagentID == "" {
			n++
		}
	}
	return n, nil
}

// Run 执行一轮 ADK ReAct 循环，返回只读事件通道（循环结束即关闭）。
// 事件同时扇出到通道与 EventStore（append-only JSONL）；Ts 在扇出时打戳。
// 底层：adk.Runner.Run → 消费 AgentEvent 流 → 翻译层映射为平台 9 种事件
// （见 events.go §4 adkTranslator）。与旧 react 路径的事件序列形状保持一致（前端零改动）。
// 调用方必须排空通道直至关闭（缓冲 256，无人消费会阻塞）。
func (a *Agent) Run(ctx context.Context, sessionID, userText string) (<-chan Event, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}
	ctx = WithSessionID(ctx, sessionID) // 工具经 SessionIDFromContext 解析会话绑定模型
	n, err := a.turnCount(sessionID)
	if err != nil {
		return nil, err
	}
	turn := n + 1 // 新开一轮

	out := make(chan Event, 256)
	// sendRaw 是唯一发送路径：落盘（EventStore）+ 扇出通道。
	sendRaw := func(ev Event) {
		if a.store != nil {
			if err := a.store.Append(sessionID, ev); err != nil && ev.Type != EventError {
				out <- Event{Type: EventError, Turn: turn, Step: ev.Step, Ts: time.Now(),
					Payload: jsonPayload(map[string]any{"error": "event store append: " + err.Error()})}
			}
		}
		out <- ev
	}

	go func() {
		sendRaw(Event{Type: EventTurnStart, Turn: turn, Payload: jsonPayload(map[string]any{"user": userText}), Ts: time.Now()})
		// 会话连续性：历史（检查阀门 BuildHistoryMessages） + 当前消息喂给模型。
		// 未超 60% 预算全量回填；超预算语义压缩（每轮指令+最终回复）。store nil（离线/测试）不喂历史。
		msgs := []*schema.Message{}
		if a.store != nil {
			if prev, err := a.store.Load(sessionID); err == nil {
				msgs = append(msgs, BuildHistoryMessages(prev, a.maxContextChars)...)
			}
		}
		msgs = append(msgs, schema.UserMessage(userText))
		// 翻译层消费 ADK 事件流并扇出（含子事件 RunPath 打标 + subagent/status 合成）；
		// Next 迭代结束（含错误/取消）后自行收尾 turn/end。
		tr := newAdkTranslator(turn, a.name, sessionID, a.maxStep, sendRaw)
		// v1 无 CheckPointStore：WithCheckPointID 仅设置 runCtx 的 checkpoint 标识，
		// 不落盘（Runner store=nil 时跳过 checkpoint 保存）。接 HITL（zref_resume）时
		// 需先给 Runner 配 CheckPointStore 再启用 ResumeWithParams。
		iter := a.runner.Run(ctx, msgs, adk.WithCheckPointID(sessionID))
		tr.run(ctx, iter)
		close(out)
	}()

	return out, nil
}

// --- HITL：CheckPointStore + Resume（2026-08-19 接线，M3） -------------------
//
// 与「会话记忆」分工（不要混）：
//   - EventStore JSONL + BuildHistoryMessages = 平台层会话记忆（历史喂模型）
//   - CheckPointStore = ADK 框架内部的中断恢复状态（StatefulInterrupt → Resume 续跑）
//
// 参考实现：eino-examples/quickstart/chatwitheino/cmd/ch09 handleInterrupt +
// eino-examples/adk/common/tool/follow_up_tool.go + store.go（in-memory）。

// memoryCheckPointStore 是 compose.CheckPointStore 的 in-memory 实现（官方 store.go 同构）：
// 存「中断时刻的 agent 运行状态」，供 ResumeWithParams 续跑。进程内有效——
// 服务重启丢中断（v1 接受；持久化留后续）。
type memoryCheckPointStore struct {
	mu  sync.Mutex
	mem map[string][]byte
}

func newMemoryCheckPointStore() *memoryCheckPointStore {
	return &memoryCheckPointStore{mem: map[string][]byte{}}
}

func (s *memoryCheckPointStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mem[checkPointID] = checkPoint
	return nil
}

func (s *memoryCheckPointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.mem[checkPointID]
	return v, ok, nil
}

// Resume 是 HITL 续跑入口：从 CheckPointStore 读中断状态，用用户回答
// （params.Targets[interruptID]）续跑 agent。checkPointID = 会话 id
// （Run 时 WithCheckPointID(sessionID) 落盘）。
// 事件流与 Run 同一翻译层（子事件打标 / question 帧 / turn 收尾）。
// turn 从 EventStore 恢复（中断发生在第 N 轮，resume 后事件仍属第 N 轮）。
func (a *Agent) Resume(ctx context.Context, sessionID string, params *adk.ResumeParams) (<-chan Event, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}
	ctx = WithSessionID(ctx, sessionID)
	n, err := a.turnCount(sessionID)
	if err != nil {
		return nil, err
	}
	turn := n
	if turn < 1 {
		turn = 1 // 中断必有 turn/start；防御兜底
	}
	out := make(chan Event, 256)
	sendRaw := func(ev Event) {
		if a.store != nil {
			_ = a.store.Append(sessionID, ev)
		}
		out <- ev
	}
	go func() {
		iter, err := a.runner.ResumeWithParams(ctx, sessionID, params)
		if err != nil {
			sendRaw(Event{Type: EventError, Turn: turn, Payload: jsonPayload(map[string]any{"error": err.Error()}), Ts: time.Now()})
			close(out)
			return
		}
		tr := newAdkTranslator(turn, a.name, sessionID, a.maxStep, sendRaw)
		tr.run(ctx, iter)
		close(out)
	}()
	return out, nil
}
