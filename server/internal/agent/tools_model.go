// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// tools_model.go：模型编辑/巡检域工具（从 tools.go 拆出，W-0057 行数门控）——
// list_models / get_model_info / get_script / stage_script / run_script /
// save_script / get_versions / get_diff。全部经 deps.resolve 做 modelId 归一 +
// kind 路由后走 edit-service REST（script-as-source 三段式 stage→run→save）。
package agent

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/cloudwego/eino/components/tool"
)

// modelTools 返回模型编辑/巡检域工具（8 个）：语义镜像 services/ifc（:8100）
// 与 services/cad（:8200）的 REST 端点；run/save/diff 走 slow client
// （沙箱执行最长 60s，fast 10s 会三方状态分叉）。
func modelTools(deps ToolDeps) []tool.InvokableTool {
	return []tool.InvokableTool{
		mustTool("list_models", "列出平台全部模型（id/名称/kind(ifc|dxf)/状态/创建时间）",
			func(ctx context.Context, _ emptyReq) (string, error) {
				if deps.St == nil {
					return "调用失败：store 未配置（list_models 不可用）", nil
				}
				ms, err := deps.St.List()
				if err != nil {
					return toolErr(err), nil
				}
				return toolJSON(ms), nil
			}),

		mustTool("get_model_info", "获取单个模型信息（id/名称/kind/状态/大小/创建时间）",
			func(ctx context.Context, in modelRefReq) (string, error) {
				m, _, errText := deps.resolve(ctx, in.ModelID)
				if errText != "" {
					return errText, nil
				}
				return toolJSON(m), nil
			}),

		mustTool("get_script", "读取模型当前构建脚本（有暂存读暂存，否则读最近大版本基线）",
			func(ctx context.Context, in modelRefReq) (string, error) {
				m, cl, errText := deps.resolve(ctx, in.ModelID)
				if errText != "" {
					return errText, nil
				}
				return toolRaw(cl.Do(ctx, http.MethodGet, "/models/"+m.ID+"/script", nil))
			}),

		mustTool("stage_script", "暂存构建脚本（全量替换；不执行）。之后必须 run_script 沙箱验证，再 save_script 落大版本",
			func(ctx context.Context, in stageScriptReq) (string, error) {
				m, cl, errText := deps.resolve(ctx, in.ModelID)
				if errText != "" {
					return errText, nil
				}
				body, _ := json.Marshal(map[string]string{"script": in.Script, "note": in.Note})
				out, err := cl.Do(ctx, http.MethodPut, "/models/"+m.ID+"/script", body)
				if err != nil {
					return toolErr(err), nil
				}
				deps.markDirty(ctx)
				return toolRaw(out, nil)
			}),

		mustTool("run_script", "沙箱执行当前暂存脚本（验证可构建并重写工作区模型文件；不落版本）",
			func(ctx context.Context, in modelRefReq) (string, error) {
				m, cl, errText := deps.resolve(ctx, in.ModelID)
				if errText != "" {
					return errText, nil
				}
				out, err := cl.DoSlow(ctx, http.MethodPost, "/models/"+m.ID+"/script/run", nil)
				if err != nil {
					return toolErr(err), nil
				}
				deps.markDirty(ctx)
				deps.pushStaged(ctx, m)
				text := truncateToolResult(string(out))
				// 摘要降级链：构件级（run 响应 semanticDiff）→ 行级 staging diff → 无摘要。
				if s := semanticDiffSummary(out); s != "" {
					text += "\n" + s
				} else if s := stagingDiffSummary(ctx, cl, m.ID); s != "" {
					text += "\n" + s
				}
				return truncateToolResult(text), nil
			}),

		mustTool("save_script", "沙箱执行并落大版本（scripts/v{n}.py + 版本快照，原子）；save 前确保已 stage_script",
			func(ctx context.Context, in saveScriptReq) (string, error) {
				m, cl, errText := deps.resolve(ctx, in.ModelID)
				if errText != "" {
					return errText, nil
				}
				body, _ := json.Marshal(map[string]string{"note": in.Note})
				out, err := cl.DoSlow(ctx, http.MethodPost, "/models/"+m.ID+"/script/save", body)
				if err != nil {
					return toolErr(err), nil
				}
				deps.markDirty(ctx)
				return toolRaw(out, nil)
			}),

		mustTool("get_versions", "列出模型的大版本（IFC 快照 versions + 构建脚本 scripts + 当前版本 current）——模型版本历史全景（参考 mcp model_versions 组合视图）",
			func(ctx context.Context, in modelRefReq) (string, error) {
				m, cl, errText := deps.resolve(ctx, in.ModelID)
				if errText != "" {
					return errText, nil
				}
				return combineModelVersions(ctx, cl, m.ID)
			}),

		mustTool("get_diff", "拉两个大版本的组合 diff：ifc=IFC 语义 diff（构件增删改）+ script=构建脚本 diff（text_diff+PARAMS 变化）——模型级版本对比全景（参考 mcp model_diff 组合视图）",
			func(ctx context.Context, in diffReq) (string, error) {
				m, cl, errText := deps.resolve(ctx, in.ModelID)
				if errText != "" {
					return errText, nil
				}
				return combineModelDiff(ctx, cl, m.ID, in.Base, in.Target)
			}),
	}
}
