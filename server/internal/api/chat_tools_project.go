// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_tools_project.go：chat 项目域（从 chat_tools.go 拆出，W-0057 行数门控）——
// create_project 后端（空白项目 + ifc/cad->ifc 管线骨架模型回滚）、项目模型聚合
// （真实状态反查，REST 详情与 agent get_project_models 工具共用同一语义源）与
// 项目详情 REST（W-0054）。
package api

import (
	"context"
	"fmt"
	"net/http"

	"ifcviewer/server/internal/store"
)

// createProjectForAgent 是 create_project 工具的后端（2026-08-20 空白化）：
// 只创建「项目」（空白，不产模型）；kind = 项目类型（ifc|cad|cad→ifc 管线）。
// 会话绑 projectId 后从空白构建（上传计划书 → 对话生成模型）。
func (h *ChatHandler) createProjectForAgent(ctx context.Context, title, kind string) (*store.Project, error) {
	if h.deps.Ps == nil {
		return nil, fmt.Errorf("create_project 未装配（项目聚合缺失）")
	}
	if title == "" {
		title = "AI 项目"
	}
	p, err := h.deps.Ps.CreateWithKind(title, kind)
	if err != nil {
		return nil, err
	}
	// ifc 管线：建项目即初始化骨架模型（分配 modelId，1 个——script-as-source：
	// 骨架脚本构建出最小 IFC v1）。骨架构建失败 → 回滚项目（不留无模型的 ifc 项目）。
	// cad/cad->ifc 管线：保持空白，agent 会话内经 init_model 工具按需初始化 DXF。
	// ifc / cad->ifc 管线：建项目即初始化 IFC 骨架模型（分配 modelId，绑定——script-as-source：
	// 骨架脚本构建出最小 IFC v1）。cad->ifc 先初始化 ifc 骨架（形成绑定），cad 部分按需
	// 经 init_model 初始化 DXF。骨架构建失败 → 回滚项目。
	// cad 管线：保持空白，agent 会话内经 init_model 按需初始化 DXF。
	if kind == store.KindIFC || kind == "cad->ifc" {
		if _, err := h.initModel(ctx, p.ID, store.KindIFC, title); err != nil {
			_ = h.deps.Ps.Delete(p.ID)
			return nil, fmt.Errorf("初始化 IFC 骨架模型: %w", err)
		}
		// initModel 经 AddModel 更新了 project.json——返回前刷新（p 是 initModel 前旧引用，
		// Models 空会让 REST 响应缺骨架模型）。
		if fresh, err := h.deps.Ps.Get(p.ID); err == nil && fresh != nil {
			return fresh, nil
		}
	}
	return p, nil
}

// createProjectForAgentTool 是 agent.ToolDeps.CreateProject 的适配：返回项目信息。
func (h *ChatHandler) createProjectForAgentTool(ctx context.Context, title, kind string) (any, error) {
	p, err := h.createProjectForAgent(ctx, title, kind)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"projectId": p.ID,
		"title":     p.Title,
		"kind":      p.Kind,
		"models":    p.Models,
	}, nil
}

// projectModelsForAgent 列项目下模型聚合（经 ProjectStore；项目不存在 → 文本错误）。
// status 反查**真实状态**（store.Get，convert 队列更新）——不依赖 Project.Models 快照
// （initModel 时硬编码 "ready"，会与实际 failed 脱节）。模型缺失（已删）跳过。
func (h *ChatHandler) projectModelsForAgent(ctx context.Context, projectID string) ([]store.ModelRef, error) {
	if h.deps.Ps == nil {
		return nil, fmt.Errorf("项目聚合未配置（store 缺失）")
	}
	p, err := h.deps.Ps.Get(projectID)
	if err != nil {
		return nil, err
	}
	return h.projectModelsWithLiveStatus(p), nil
}

// projectModelsWithLiveStatus 从已取回的项目聚合刷新模型真实状态（单一语义源：
// agent get_project_models 工具与 REST 项目详情共用——每项 {id,kind,name,status}，
// status 反查 store.Get；模型缺失（已删）保留快照引用）。
func (h *ChatHandler) projectModelsWithLiveStatus(p *store.Project) []store.ModelRef {
	out := make([]store.ModelRef, 0, len(p.Models))
	for _, ref := range p.Models {
		cur := ref
		if h.deps.St != nil {
			if m, err := h.deps.St.Get(ref.ID); err == nil {
				cur.Status = m.Status
			}
		}
		out = append(out, cur)
	}
	return out
}

// getProject 项目详情 REST（W-0054）：project 元信息 + 项目下模型列表。
// 响应 data 形状与 POST /chat/projects（create）一致——{projectId,title,kind,
// createdAt,models}；models 与 agent get_project_models 工具同语义（真实状态反查），
// 前端/agent 不出现两套字段语义。404/装配缺失归 projectOrErr helper（翻译层）。
func (h *ChatHandler) getProject(w http.ResponseWriter, r *http.Request) {
	p := h.projectOrErr(w, r.PathValue("id"))
	if p == nil {
		return
	}
	writeJSON(w, map[string]any{
		"projectId": p.ID,
		"title":     p.Title,
		"kind":      p.Kind,
		"createdAt": p.CreatedAt,
		"models":    h.projectModelsWithLiveStatus(p),
	})
}
