// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// agents.go：ADK 三角色装配（路线 B，D10 全面迁移）。
//
// orchestrator（主 agent）经官方 AgentAsTool（adk.NewAgentTool）派发 ifc/cad
// 子 agent——子 agent 是独立 ChatModelAgent（Name/Description/各自 persona/
// 独立模型实例/领域工具 + skill middleware），由 orchestrator 的 Tools 装配。
// EmitInternalEvents=true 让子 AgentEvent 实时上浮，翻译层按 RunPath 合成
// subagentId 标签与 subagent/status（见 events.go §4）。
//
// orchestrator 特殊性（D11）：它是唯一对话入口，aiplan/aibim-orchestrator
// 的对话协调层内联在 orchestrator（skill middleware 全量挂载，模型可加载
// aiplan 规划流程 + aibim-orchestrator 编排手册）；子 agent 各自挂领域 skill
// （aiifc / aidxf，当前为全量目录，角色化过滤留后续精化）。
package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	localbk "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/cloudwego/eino/adk/middlewares/skill"

	fsmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
)

// OrchestratorPersona 是主 agent 的编排者人格（要点取自 skills/aibim-orchestrator
// SKILL.md 意图路由表 + 派发纪律）：主 agent 不直接建模/画图，判断意图 → 派发
// 子 agent → 汇总回报；设计规范/审查问答直接回答不派发。
//
// 路由纪律（第一层：意图 × skill 组合）：
//   - CAD/DXF 生成修改 → **必须先加载 aiplan** 与用户对话框定方案，再派 cad-agent（强前置）
//   - IFC 生成修改 → 直接派 ifc-agent（独立，不经 aiplan；ifc↔dxf 产物未对接，两条线独立）
//   - 模糊想法/完整方案 → aiplan → cad →（ifc 可选，全链）
//   - 设计规范/审查问答 → 直接回答，不派发
//
// OrchestratorPersona 是 cad->ifc 项目的编排者人格（全装：aiplan + cad + ifc——
// cad->ifc 管线需要三个都装）。**kind 强制必选（create_project 强制 ifc|cad|cad->ifc，
// 无空 kind）**——orchestrator 按 kind 强制装配：cad→personaCAD、ifc→personaIFC、
// cad->ifc→OrchestratorPersona（本 persona，专用于 cad->ifc 全链编排）。
const OrchestratorPersona = `你是 SourceAEC cad->ifc 项目的编排者，按需要协调规划、CAD 与 IFC 建模。
先明确目标、输入和交付物。复杂规划可记录为 plan.json 与 bim_supplement.json，调用 deliver_plan 由 Go 保存。
需要 CAD 时派 cad-agent，需要 IFC 时派 ifc-agent；子任务应携带需求、projectID、modelId 和现有输入。
CAD 到 IFC 的输入可通过 stage_upstream_to_workdir 获取；不要臆造旧 CLI 或 SDK。
所有模型修改走 get_script → stage_script（baseRevision）→ run_script → save_script。
按实际运行结果报告版本、检查证据和未验证项。设计规范问答可以直接回答。`

const personaCAD = `你是 SourceAEC CAD 项目的协调者。按需要使用 aiplan 声明规划输入，调用 deliver_plan 保存。
已有规划可通过 stage_plan_to_workdir 读取，派 cad-agent 在现有 modelId 上增量修改。
需要新图纸时使用 init_model；交付 building.json 用 deliver_building。只按实际检查结果报告。`
const personaIFC = `你是 SourceAEC IFC 项目的协调者。先核对设计意图，再派 ifc-agent 深化项目已有骨架。
复杂设计可以用声明性 JSON 记录需求；没有强制设计 DSL 或 Python CLI 前置。
模型修改使用脚本 API，检查实际 IFC 后保存。按验证结果报告，不臆造合规结论。`
const (
	PersonaIFC      = "ifc-agent"
	PersonaCAD      = "cad-agent"
	ifcAgentPersona = `你是 IFC 建模 Agent。读取已有 get_script，使用外部 ifcopenshell 库编写运行时模型源码。
顶层 PARAMS 使用字面量字典。程序接收输出路径作为第一个参数；不要导入本仓 script_lib 或调用 aiifc CLI。
修改使用 stage_script（baseRevision）→ run_script → save_script；优先增量修改并保持 GlobalId 和 PARAMS key 稳定。
可用 get_project_plans 或 stage_upstream_to_workdir 获取上游声明性输入。业务编排与发布由 Go 负责。
只向主 Agent 报告实际产物、版本、验证证据和未解决问题。`
	cadAgentPersona = `你是 CAD 建模 Agent。使用已有规划输入与 modelId，通过外部 ezdxf 或配置好的几何库生成运行时源码。
顶层 PARAMS 为字面量字典；输出路径是第一个参数。不要调用本仓规划或制图 CLI、导入本仓画法库。
使用 get_script → stage_script（baseRevision）→ run_script → save_script，检查单位、尺寸、图层和实际输出。
需要新图纸使用 init_model。可选实体稳定 key 用 AIDXF/SOURCEAEC XDATA；不要把业务逻辑搬入执行脚本。
只向主 Agent 报告实际产物、版本、检查证据和遗留问题。`
)

// newSkillMiddleware 构造官方 skill middleware（ch09 同构：local backend →
// skill backend 扫 BaseDir/*/SKILL.md → NewTyped）。skillsDir 为空返回 nil。
// allow 非空时按角色过滤：模型工具面只出现允许的 skill（第一层：角色映射）。
func newSkillMiddleware(ctx context.Context, skillsDir string, allow ...string) (adk.TypedChatModelAgentMiddleware[*schema.Message], error) {
	if skillsDir == "" {
		return nil, nil
	}
	backend, err := localbk.NewBackend(ctx, &localbk.Config{})
	if err != nil {
		return nil, fmt.Errorf("create local backend: %w", err)
	}
	skillBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: backend,
		BaseDir: skillsDir,
	})
	if err != nil {
		return nil, fmt.Errorf("create skill backend: %w", err)
	}
	if len(allow) > 0 {
		skillBackend = &filteredSkillBackend{inner: skillBackend, allow: toSet(allow)}
	}
	skillMW, err := skill.NewTyped[*schema.Message](ctx, &skill.TypedConfig[*schema.Message]{
		Backend: skillBackend,
	})
	if err != nil {
		return nil, fmt.Errorf("create skill middleware: %w", err)
	}
	return skillMW, nil
}

// filteredSkillBackend 是 skill.Backend 的角色过滤薄包装：
// List 只返回允许集内的 skill（工具描述只显示本角色可用的）；
// Get 拒绝允许集外的调用（模型调别的 skill → 文本错误，不中断循环）。
type filteredSkillBackend struct {
	inner skill.Backend
	allow map[string]bool
}

func toSet(names []string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func (b *filteredSkillBackend) List(ctx context.Context) ([]skill.FrontMatter, error) {
	all, err := b.inner.List(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, fm := range all {
		if b.allow[fm.Name] {
			out = append(out, fm)
		}
	}
	return out, nil
}

func (b *filteredSkillBackend) Get(ctx context.Context, name string) (skill.Skill, error) {
	if !b.allow[name] {
		return skill.Skill{}, fmt.Errorf("skill 不属于当前角色（仅允许 %s）：%s", keysOf(b.allow), name)
	}
	return b.inner.Get(ctx, name)
}

func keysOf(m map[string]bool) string {
	var names []string
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, "/")
}

// newFilesystemMiddleware 构造收敛版官方 filesystem middleware（M2-0/D12）：
//   - Backend = 读透传 + Write/Edit **白名单放开到 skill 工作区**（skillWorkRoot={DATA}/skill-work，
//     agent 可写 design.json 等 LLM 意图中间产物；其它路径拒绝）
//   - StreamingShell = local backend + ValidateCommand 命令白名单（aiplan/aidxfv3/aiifc）
//
// 模型由此获得：read_file/glob/grep/ls（读 skill references）+ write_file（仅 skill-work）+
// execute（跑 skill CLI）。
func newFilesystemMiddleware(ctx context.Context, skillWorkRoot string, readRoots ...string) (adk.TypedChatModelAgentMiddleware[*schema.Message], error) {
	backend, err := localbk.NewBackend(ctx, &localbk.Config{
		ValidateCommand: validateSkillCommand, // 命令白名单（领域收敛单点）
	})
	if err != nil {
		return nil, fmt.Errorf("create local backend: %w", err)
	}
	mw, err := fsmw.NewTyped[*schema.Message](ctx, &fsmw.MiddlewareConfig{
		Backend:        &fsReadOnlyBackend{inner: backend, skillWorkRoot: skillWorkRoot, readRoots: append(readRoots, skillWorkRoot)},
		StreamingShell: &artifactShell{inner: backend, root: skillWorkRoot}, // persist CLI outputs
	})
	if err != nil {
		return nil, fmt.Errorf("create filesystem middleware: %w", err)
	}
	return mw, nil
}

// roleAgentConfig 是子 agent（被 AgentAsTool 派发的角色）装配参数。
type roleAgentConfig struct {
	name        string
	description string
	instruction string
	model       model.ToolCallingChatModel
	tools       []tool.BaseTool
	skillsDir   string
	skills      []string // 本角色允许的 skill 名（角色化过滤，第一层）；空 = 不挂 skill
	maxStep     int
	dataDir     string // 平台数据根（{DATA}）；filesystem Write 白名单根 = {DATA}/skill-work
}

// newRoleAgent 构造一个角色子 agent：独立 ChatModelAgent（Name/Description/
// Instruction/独立模型实例）+ 领域工具 + 角色 skill middleware + filesystem middleware
// + 工具错误兜底。不含 AgentAsTool——深度预算 1 结构性保证（子 agent 无派发工具）。
func newRoleAgent(ctx context.Context, cfg roleAgentConfig) (adk.Agent, error) {
	var handlers []adk.TypedChatModelAgentMiddleware[*schema.Message]
	if cfg.skillsDir != "" && len(cfg.skills) > 0 {
		skillMW, err := newSkillMiddleware(ctx, cfg.skillsDir, cfg.skills...)
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, skillMW)
	}
	fsMW, err := newFilesystemMiddleware(ctx, skillWorkRootFor(cfg.dataDir), cfg.skillsDir)
	if err != nil {
		return nil, err
	}
	handlers = append(handlers, fsMW)
	handlers = append(handlers, newSafeToolMiddleware())
	ag, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          cfg.name,
		Description:   cfg.description,
		Instruction:   cfg.instruction,
		Model:         cfg.model,
		MaxIterations: cfg.maxStep,
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: cfg.tools,
		}},
		Handlers: handlers,
	})
	if err != nil {
		return nil, fmt.Errorf("create role agent %s: %w", cfg.name, err)
	}
	return ag, nil
}

// kindChildren 按项目类型选择 AgentAsTool 子 agent：
//
//	cad->ifc/空（全装）→ cad + ifc；cad → 只 cad；ifc → 只 ifc。
func kindChildren(kind string, cad, ifc adk.Agent) []adk.Agent {
	var out []adk.Agent
	if kind != "ifc" {
		out = append(out, cad)
	}
	if kind != "cad" {
		out = append(out, ifc)
	}
	return out
}

// orchestratorTools 把子 agent 包装为官方 AgentAsTool 并追加到父工具面。
func orchestratorTools(tools []tool.BaseTool, children ...adk.Agent) []tool.BaseTool {
	out := make([]tool.BaseTool, 0, len(tools)+len(children))
	out = append(out, tools...)
	for _, child := range children {
		out = append(out, adk.NewAgentTool(context.Background(), child))
	}
	return out
}
