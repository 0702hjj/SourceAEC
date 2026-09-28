// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// tools_role.go：工具面角色过滤——A2 工具面按角色分离。
// ifc-agent 剔除 init_model（ifc 骨架在 create_project 已绑定，1 项目 = 1 ifc 模型，
// 项目创建即分配 modelId + 绑 projectId），agent 只能在绑定模型上深化 stage/run/save，
// 禁止再建新 ifc；cad-agent 保留（cad 项目空白，按需 init_model 建 DXF）。
package agent

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
)

// AsBaseTools 把 DomainTools 产出转为 WithTools 的入参形状。
func AsBaseTools(ts []tool.InvokableTool) []tool.BaseTool {
	out := make([]tool.BaseTool, len(ts))
	for i, t := range ts {
		out[i] = t
	}
	return out
}

// FilterRoleTools 按角色过滤工具面（返回新切片，不改原）。
// 装配侧结构性剔除（同「深度预算 1」哲学）：模型工具面物理上无该工具 → 调用必然
// 「未知工具」→ 模型自愈走既有路径，而不是被诱导新建模型。
func FilterRoleTools(tools []tool.BaseTool, exclude ...string) []tool.BaseTool {
	block := map[string]bool{}
	for _, e := range exclude {
		block[e] = true
	}
	out := make([]tool.BaseTool, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil || block[info.Name] {
			continue
		}
		out = append(out, t)
	}
	return out
}
