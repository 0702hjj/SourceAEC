// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// tools.go：agent 领域工具注册核心——会话 ctx 注入（WithSessionID）、
// ToolDeps 依赖包、模型归一守卫（resolve）、mustTool 构造器与 DomainTools 总装。
// 各域工具本体（W-0057 行数门控拆出）：tools_model.go（模型编辑/巡检）、
// tools_project.go（项目/方案）、tools_locate.go（选中定位/标量改写）、
// tools_askuser.go（HITL 提问）；通用 helper 与请求 schema 见 tools_helpers.go，
// 工具面角色过滤见 tools_role.go。
package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"ifcviewer/server/internal/editsvc"
	"ifcviewer/server/internal/store"
)

type ctxKeySessionID struct{}

// WithSessionID 把会话 id 注入 ctx（Run 在启动 ReAct 循环前调用；
// 测试/装配侧可用它手工构造带会话上下文的 ctx）。
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, ctxKeySessionID{}, sessionID)
}

// SessionIDFromContext 取出 Run 注入的会话 id（工具经它解析会话绑定模型）。
func SessionIDFromContext(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeySessionID{}).(string)
	return s
}

// ToolDeps 是领域工具集的依赖包。工具面领域收敛：不持有 bash/任意文件写能力，
// 全部变更经 edit-service（ifc）/ cad-service（dxf）REST，按模型 kind 路由。
type ToolDeps struct {
	IFC *editsvc.Client // ifc kind 后端（services/ifc :8100）
	CAD *editsvc.Client // dxf kind 后端（services/cad :8200）；nil 时 dxf 工具报错文本
	St  *store.Store    // kind 路由 + list/get model info

	// SessionModel 从 ctx 解析当前会话绑定的模型 id（无绑定返回 ""）；可空。
	SessionModel func(ctx context.Context) string
	// MarkDirty 在变更类工具成功后标记会话 dirty（notify 精确信号，不再只靠 mtime）；
	// create_project 不置位（新模型与绑定模型是两个对象，置位会让 notify 错绑管线）；可空。
	MarkDirty func(ctx context.Context)
	// PushStaged 在 run_script 成功后推送 viewer.staged 中途预览信号
	// （{modelId, kind} 载荷，走 pushSystem 管线；同 MarkDirty 的可空适配器模式）；可空。
	PushStaged func(ctx context.Context, modelID, kind string)
	// CreateProject 创建「项目」（项目级 A1：projectID + 首交付模型，kind ifc|dxf）；
	// 返回可 JSON 化的 {model, project}；可空。
	CreateProject func(ctx context.Context, title, kind string) (any, error)
	// InitModel 在项目下初始化骨架模型（agent 会话内新建 DXF/IFC——script-as-source：
	// 骨架脚本构建出最小模型 v1）。返回 {modelId, kind, title, projectId}；可空。
	InitModel func(ctx context.Context, projectID, kind, title string) (any, error)

	// --- D2 项目/方案域（交付对齐） ---
	// SessionProject 从 ctx 解析会话绑定项目 id（A2；无绑定返回 ""）；可空。
	SessionProject func(ctx context.Context) string
	// ProjectModels 列项目下模型聚合（id/kind/name/status）；可空。
	ProjectModels func(ctx context.Context, projectID string) ([]store.ModelRef, error)
	// PlanGet 读方案产物当前态（name = plan.json|bim_supplement.json）；可空。
	PlanGet func(ctx context.Context, projectID, name string) (string, error)
	// PlanDeliver 触发 plan 交付（B2：aiplan land → 落方案级目录），
	// 返回 {planVersion, bimVersion}；可空。
	PlanDeliver func(ctx context.Context, projectID, plan, bimSupplement string) (map[string]any, error)
	// BuildingDeliver 交付 building.json（aidxf S4-c：agent 组装 plan 形态整栋楼 + zones 记
	// modelId）→ PlanStore 版本化 plans/{projectID}/building.json，返回 {buildingVersion}；可空。
	BuildingDeliver func(ctx context.Context, projectID, building string) (map[string]any, error)
	// SkillWorkDir 返回项目 skill 工作区绝对路径（{DATA}/skill-work/{projectID}，
	// 首次调用 MkdirAll；projectId 隔离多项目不混淆）——aidxf 中间产物
	// （derived/missions/deliver）落盘根，复用 plans/{projectID} 的 projectId 隔离地基；可空。
	SkillWorkDir func(ctx context.Context, projectID string) (string, error)
	// PlanToWorkdir 把项目 plan 产物（plan.json + bim_supplement.json）从 PlanStore
	// 落到 skill 工作区文件，返回 {planPath, bimPath}——aidxfv3 preprocess --plan 等命令
	// 需要文件路径时的桥接（plan 内容 → 工作区文件）；可空。
	PlanToWorkdir func(ctx context.Context, projectID string) (map[string]string, error)
	// UpstreamToWorkdir 把 ifc 消费的上游产物（building.json + bim_supplement.json +
	// 各 zone DXF）落到 skill 工作区，返回 {buildingPath, bimPath, dxfDir, dxfPaths}——
	// cad->ifc 消费上游：ifc-agent 跑 aiifc consume-upstream 的输入桥接（多 DXF 按
	// zones[].modelId 从 uploads/{modelId}.dxf 复制到工作区 dxf/）；可空。
	UpstreamToWorkdir func(ctx context.Context, projectID string) (map[string]any, error)
}

func (d ToolDeps) markDirty(ctx context.Context) {
	if d.MarkDirty != nil {
		d.MarkDirty(ctx)
	}
}

func (d ToolDeps) pushStaged(ctx context.Context, m *store.Model) {
	if d.PushStaged != nil {
		d.PushStaged(ctx, m.ID, m.Kind)
	}
}

// resolve 归一 modelId（参数缺省回退会话绑定模型）并做 kind 路由。
// 失败返回非空 errText——工具把错误以文本返回供 LLM 观测自愈（sec-agent 模式），
// 非法/未知 modelId 在此拦截，不会触达任何后端（守卫）。
func (d ToolDeps) resolve(ctx context.Context, modelID string) (*store.Model, *editsvc.Client, string) {
	if modelID == "" && d.SessionModel != nil {
		modelID = d.SessionModel(ctx)
	}
	if modelID == "" {
		return nil, nil, "未指定 modelId，且当前会话未绑定模型——请先 create_project 或在绑定模型的会话中重试"
	}
	if d.St == nil {
		return nil, nil, "调用失败：store 未配置（模型工具不可用）"
	}
	m, err := d.St.Get(modelID)
	if err != nil {
		return nil, nil, truncateToolResult(fmt.Sprintf("模型 %q 不可用：%v", modelID, err))
	}
	cl := d.IFC
	if m.Kind == store.KindDXF {
		cl = d.CAD
	}
	if cl == nil {
		return nil, nil, truncateToolResult(fmt.Sprintf("模型 %s 的后端未配置（kind=%s）", modelID, m.Kind))
	}
	return m, cl, ""
}

// mustTool 构造 InferTool；schema 是静态的，构造失败即程序员错误，启动期直接 panic
// （同 http.ServeMux 冲突 panic 语义），不拖错误签名污染 DomainTools 契约。
func mustTool[T, R any](name, desc string, fn func(context.Context, T) (R, error)) tool.InvokableTool {
	tl, err := utils.InferTool(name, desc, fn)
	if err != nil {
		panic(fmt.Sprintf("domain tool %s: %v", name, err))
	}
	return tl
}

// DomainTools 装配 chat agent 的领域工具集（9 个）：
// list_models / get_model_info / get_script / stage_script / run_script /
// save_script / get_versions / get_diff / create_project。
// 语义镜像 services/ifc（:8100）与 services/cad（:8200）的 REST 端点；
// run/save/diff 走 slow client（沙箱执行最长 60s，fast 10s 会三方状态分叉）。
// 分域装配（W-0057 拆分）：modelTools + projectTools + locateTools 顺序拼接，
// 注册顺序与拆分前的单一切片完全一致。
func DomainTools(deps ToolDeps) []tool.InvokableTool {
	ts := append(append([]tool.InvokableTool{}, modelTools(deps)...), projectTools(deps)...)
	return append(ts, locateTools(deps)...)
}
