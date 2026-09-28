// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// tools_project.go：项目/方案域工具（从 tools.go 拆出，W-0057 行数门控）——
// create_project / init_model（项目与骨架模型创建）+ get_project_plans /
// deliver_plan / deliver_building（方案产物读写与交付）+ get_project_models /
// get_skill_workdir / stage_plan_to_workdir / stage_upstream_to_workdir
// （项目聚合与 skill 工作区桥接）。统一经 resolveProjectID 归一项目 id。
package agent

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/tool"
)

// projectTools 返回项目/方案域工具（9 个，D2 交付对齐）。
func projectTools(deps ToolDeps) []tool.InvokableTool {
	return []tool.InvokableTool{
		mustTool("create_project", "创建空白项目（项目级：projectID + 首交付模型，kind 可选 ifc/dxf 默认 ifc），返回首模型（modelId 供后续编辑）+ projectId（会话绑定用它）",
			func(ctx context.Context, in createProjectReq) (string, error) {
				if deps.CreateProject == nil {
					return "create_project 未配置（装配缺失）", nil
				}
				v, err := deps.CreateProject(ctx, in.Title, in.Kind)
				if err != nil {
					return toolErr(err), nil
				}
				// 不 markDirty：新模型 B 与会话绑定的模型 A 是两个对象——置 dirty 会让
				// turn 结束的 notifyIfDirty 对未变更的 A 跑完整管线（stale staging 时
				// save 出无意图版本）。新模型的转换由 CreateProject 内部 Enqueue 直接触发，
				// 不依赖 notify；后续对 B 的 stage/run/save 才会置 dirty。
				return toolJSON(v), nil
			}),

		mustTool("init_model", "在项目下初始化骨架模型（新建 DXF/IFC：骨架脚本沙箱构建出最小模型 v1，分配 modelId）——agent 会话内新建模型的入口；CAD 项目每次新建图纸时调用，后续看 get_project_models 决定新建或编辑已有",
			func(ctx context.Context, in initModelReq) (string, error) {
				if deps.InitModel == nil {
					return "init_model 未配置（装配缺失）", nil
				}
				pid := in.ProjectID
				if pid == "" {
					pid = resolveProjectID(ctx, deps, "")
				}
				kind := in.Kind
				if kind == "" {
					kind = "dxf"
				}
				v, err := deps.InitModel(ctx, pid, kind, in.Title)
				if err != nil {
					return toolErr(err), nil
				}
				deps.markDirty(ctx)
				return toolJSON(v), nil
			}),

		mustTool("get_project_plans", "读项目方案产物当前态（plan.json / bim_supplement.json / building.json）——plan 是任务书（对接 cad），bim_supplement 是 BIM 补充（对接 bim），building 是 aidxf 交付的整栋楼（zones 记 modelId，对接 ifc；早期无 building 时不含该字段）",
			func(ctx context.Context, in projectRefReq) (string, error) {
				projectID := resolveProjectID(ctx, deps, in.ProjectID)
				if projectID == "" {
					return "未指定 projectId，且当前会话未绑定项目——请先 create_project 或在绑定项目的会话中重试", nil
				}
				if deps.PlanGet == nil {
					return "get_project_plans 未配置（装配缺失）", nil
				}
				plan, err := deps.PlanGet(ctx, projectID, "plan.json")
				if err != nil {
					return toolErr(err), nil
				}
				bim, err := deps.PlanGet(ctx, projectID, "bim_supplement.json")
				if err != nil {
					return toolErr(err), nil
				}
				out := map[string]any{
					"projectId": projectID, "plan": json.RawMessage(plan), "bimSupplement": json.RawMessage(bim),
				}
				// building.json 容忍缺失（aidxf S4-c 交付后才有；缺失时不含该字段，不报错）。
				if building, berr := deps.PlanGet(ctx, projectID, "building.json"); berr == nil {
					out["building"] = json.RawMessage(building)
				}
				return toolJSON(out), nil
			}),

		mustTool("deliver_plan", "执行 plan 交付（aiplan land → 方案级目录版本化）：body 传 plan + bimSupplement（可从 get_project_plans 读后修改再交）",
			func(ctx context.Context, in deliverPlanReq) (string, error) {
				projectID := resolveProjectID(ctx, deps, in.ProjectID)
				if projectID == "" {
					return "未指定 projectId，且当前会话未绑定项目——请先 create_project 或在绑定项目的会话中重试", nil
				}
				if deps.PlanDeliver == nil {
					return "deliver_plan 未配置（装配缺失）", nil
				}
				v, err := deps.PlanDeliver(ctx, projectID, string(in.Plan), string(in.BimSupplement))
				if err != nil {
					return toolErr(err), nil
				}
				return toolJSON(v), nil
			}),

		mustTool("deliver_building", "交付 building.json（aidxf S4-c：你组装 plan 形态整栋楼 + zones 记 modelId——读 get_project_plans 的 plan.json + 各 zone init_model 的 modelId 组装）→ 方案库版本化 plans/{projectID}/building.json。building 不由 CLI deliver 产（它不知道你的 modelId）",
			func(ctx context.Context, in deliverBuildingReq) (string, error) {
				projectID := resolveProjectID(ctx, deps, in.ProjectID)
				if projectID == "" {
					return "未指定 projectId，且当前会话未绑定项目——请先 create_project 或在绑定项目的会话中重试", nil
				}
				if deps.BuildingDeliver == nil {
					return "deliver_building 未配置（装配缺失）", nil
				}
				v, err := deps.BuildingDeliver(ctx, projectID, string(in.Building))
				if err != nil {
					return toolErr(err), nil
				}
				return toolJSON(v), nil
			}),

		mustTool("get_project_models", "列项目下模型聚合（id/kind/name/status）——项目会话内查看全部交付模型",
			func(ctx context.Context, in projectRefReq) (string, error) {
				projectID := resolveProjectID(ctx, deps, in.ProjectID)
				if projectID == "" {
					return "未指定 projectId，且当前会话未绑定项目——请先 create_project 或在绑定项目的会话中重试", nil
				}
				if deps.ProjectModels == nil {
					return "get_project_models 未配置（装配缺失）", nil
				}
				models, err := deps.ProjectModels(ctx, projectID)
				if err != nil {
					return toolErr(err), nil
				}
				return toolJSON(map[string]any{"projectId": projectID, "models": models}), nil
			}),

		mustTool("get_skill_workdir", "返回项目 skill 工作区绝对路径（{DATA}/skill-work/{projectID}，首次调用自动建目录；projectId 隔离多项目不混淆）——aidxf 中间产物（derived/missions/deliver）落盘根：所有 aidxfv3 命令的 --project 必须用它",
			func(ctx context.Context, in projectRefReq) (string, error) {
				projectID := resolveProjectID(ctx, deps, in.ProjectID)
				if projectID == "" {
					return "未指定 projectId，且当前会话未绑定项目——请先 create_project 或在绑定项目的会话中重试", nil
				}
				if deps.SkillWorkDir == nil {
					return "get_skill_workdir 未配置（装配缺失）", nil
				}
				dir, err := deps.SkillWorkDir(ctx, projectID)
				if err != nil {
					return toolErr(err), nil
				}
				return toolJSON(map[string]any{"projectId": projectID, "workdir": dir}), nil
			}),

		mustTool("stage_plan_to_workdir", "把项目 plan 产物（plan.json + bim_supplement.json）从方案库落到 skill 工作区文件，返回 {planPath, bimPath}——aidxfv3 preprocess --plan <文件> 等命令需要文件路径时调用（plan 内容 → 工作区文件的桥接）。先 get_project_plans 确认 plan 存在再调",
			func(ctx context.Context, in projectRefReq) (string, error) {
				projectID := resolveProjectID(ctx, deps, in.ProjectID)
				if projectID == "" {
					return "未指定 projectId，且当前会话未绑定项目——请先 create_project 或在绑定项目的会话中重试", nil
				}
				if deps.PlanToWorkdir == nil {
					return "stage_plan_to_workdir 未配置（装配缺失）", nil
				}
				paths, err := deps.PlanToWorkdir(ctx, projectID)
				if err != nil {
					return toolErr(err), nil
				}
				return toolJSON(map[string]any{
					"projectId": projectID,
					"planPath":  paths["planPath"],
					"bimPath":   paths["bimPath"],
				}), nil
			}),

		mustTool("stage_upstream_to_workdir", "把 ifc 消费的上游产物（building.json + bim_supplement.json + 各 zone DXF）落到 skill 工作区，返回 {buildingPath, bimPath, dxfDir, dxfPaths}——cad->ifc 消费上游：你跑 aiifc consume-upstream --building <buildingPath> --bim <bimPath> --dxf-dir <dxfDir> 前的输入桥接（多 DXF 按 zones[].modelId 从平台模型复制到工作区 dxf/）。先 get_project_plans 确认 building 存在再调",
			func(ctx context.Context, in projectRefReq) (string, error) {
				projectID := resolveProjectID(ctx, deps, in.ProjectID)
				if projectID == "" {
					return "未指定 projectId，且当前会话未绑定项目——请先 create_project 或在绑定项目的会话中重试", nil
				}
				if deps.UpstreamToWorkdir == nil {
					return "stage_upstream_to_workdir 未配置（装配缺失）", nil
				}
				v, err := deps.UpstreamToWorkdir(ctx, projectID)
				if err != nil {
					return toolErr(err), nil
				}
				return toolJSON(v), nil
			}),
	}
}

// resolveProjectID 归一项目 id（参数缺省回退会话绑定项目）。
func resolveProjectID(ctx context.Context, deps ToolDeps, projectID string) string {
	if projectID == "" && deps.SessionProject != nil {
		projectID = deps.SessionProject(ctx)
	}
	return projectID
}
