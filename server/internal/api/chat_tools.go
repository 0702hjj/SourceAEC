// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_tools.go：agent 领域工具集在 chat 模块的装配——ToolDeps 适配（会话绑定
// 模型解析 / dirty 精确信号 / staged 中途预览信号）。
// 工具本体与路由守卫见 internal/agent/tools.go；本文件只做 deps 的胶水与锁纪律。
// 项目域后端见 chat_tools_project.go；方案/工作区域见 chat_tools_plan.go
// （W-0057 行数门控拆出）。
package api

import (
	"context"

	"github.com/cloudwego/eino/components/tool"

	"ifcviewer/server/internal/agent"
)

// markSessionDirty 把会话置 dirty（变更类工具成功后调用）：notify 不再只靠
// uploads mtime 兜底——工具明确报告了变更落地。持 h.mu 写锁。
func (h *ChatHandler) markSessionDirty(ctx context.Context) {
	sid := agent.SessionIDFromContext(ctx)
	if sid == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if cid, ok := h.byAgent[sid]; ok {
		if cs := h.sessions[cid]; cs != nil {
			cs.dirty = true
		}
	}
}

// pushStaged 是 run_script 工具的中途预览信号适配器：会话解析同 markSessionDirty
// （agentSessionId → chatSessionId），经 pushSystem 推 viewer.staged——载荷严格
// {modelId, kind}，主会话事件（无 subagentId）。pushSystem 自持锁，此处只读解析。
func (h *ChatHandler) pushStaged(ctx context.Context, modelID, kind string) {
	sid := agent.SessionIDFromContext(ctx)
	if sid == "" {
		return
	}
	h.mu.RLock()
	cid, ok := h.byAgent[sid]
	h.mu.RUnlock()
	if !ok {
		return
	}
	h.pushSystem(cid, "viewer.staged", map[string]any{"modelId": modelID, "kind": kind})
}

// sessionBoundModel 解析 ctx 会话绑定的 modelId（agentSessionId → chatSession →
// ModelID）；无绑定返回 ""（工具面提示模型上下文缺失）。
func (h *ChatHandler) sessionBoundModel(ctx context.Context) string {
	sid := agent.SessionIDFromContext(ctx)
	if sid == "" {
		return ""
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	cid, ok := h.byAgent[sid]
	if !ok {
		return ""
	}
	if cs := h.sessions[cid]; cs != nil {
		return cs.ModelID
	}
	return ""
}

// AgentToolDeps 组装 chat agent 的领域工具依赖（main/测试装配共用）。
func (h *ChatHandler) AgentToolDeps() agent.ToolDeps {
	return agent.ToolDeps{
		IFC:           h.deps.Ed,
		CAD:           h.deps.Cad,
		St:            h.deps.St,
		SessionModel:  h.sessionBoundModel,
		MarkDirty:     h.markSessionDirty,
		PushStaged:    h.pushStaged,
		CreateProject: h.createProjectForAgentTool,
		InitModel:     h.initModelForAgentTool,
		// D2 项目/方案域
		SessionProject:    h.sessionBoundProject,
		ProjectModels:     h.projectModelsForAgent,
		PlanGet:           h.planGetForAgent,
		PlanDeliver:       h.planDeliverForAgent,
		BuildingDeliver:   h.buildingDeliverForAgent,
		SkillWorkDir:      h.skillWorkDirForAgent,
		PlanToWorkdir:     h.planToWorkdirForAgent,
		UpstreamToWorkdir: h.upstreamToWorkdirForAgent,
	}
}

// sessionBoundProject 解析 ctx 会话绑定的项目 id（A2；无绑定返回 ""）。
func (h *ChatHandler) sessionBoundProject(ctx context.Context) string {
	sid := agent.SessionIDFromContext(ctx)
	if sid == "" {
		return ""
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	cid, ok := h.byAgent[sid]
	if !ok {
		return ""
	}
	if cs := h.sessions[cid]; cs != nil {
		return cs.ProjectID
	}
	return ""
}

// DomainTools 产出 chat agent 的领域工具集（main 装配：agent.WithTools）。
func (h *ChatHandler) DomainTools() []tool.BaseTool {
	return agent.AsBaseTools(agent.DomainTools(h.AgentToolDeps()))
}

// SetAgent 回填默认 agent（main 装配顺序：handler 先建（工具 deps 需要会话表回调）、
// agent 后建（注入工具）、最后回填引用破环）。启动后不应再改。
func (h *ChatHandler) SetAgent(ag *agent.Agent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deps.Ag = ag
}

// SetAgents 回填按项目类型分化的主 agent 集（kind → agent）；启动后不应再改。
func (h *ChatHandler) SetAgents(agents map[string]*agent.Agent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deps.Agents = agents
}

// agentForSession 按会话绑定的项目类型路由主 agent：
//   - 项目会话（ProjectID）→ Project.Kind → Agents[kind]；未分化（缺 map/该 kind）落默认 Ag
//   - 模型会话/无绑定 → 默认 Ag
//
// 历史项目会话（重启后从 chat-sessions.json 恢复）同样按 ProjectID 命中——kind
// 决定 AgentAsTool 选择性装配 + persona + aiplan，会话恢复不落回默认全装。
func (h *ChatHandler) agentForSession(cs *chatSession) *agent.Agent {
	if cs != nil && cs.ProjectID != "" {
		if h.deps.Ps != nil {
			if p, err := h.deps.Ps.Get(cs.ProjectID); err == nil && p != nil && p.Kind != "" {
				if ag := h.deps.Agents[p.Kind]; ag != nil {
					return ag
				}
			}
		}
	}
	return h.deps.Ag
}
