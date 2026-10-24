// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// agents.go：ADK 三角色装配（路线 B，D10 全面迁移）。
//
// orchestrator（主 agent）经官方 AgentAsTool（adk.NewAgentTool）派发 ifc/cad
// 子 agent——子 agent 是独立 ChatModelAgent（Name/Description/各自 persona/
// 独立模型实例/领域工具 + skill middleware），由 orchestrator 的 Tools 装配。
// EmitInternalEvents=true 让子 AgentEvent 实时上浮，翻译层按 RunPath 合成
// subagentId 标签与 subagent/status（见 events.go §4）。
package agent

import (
	"context"
	"fmt"

	localbk "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/cloudwego/eino/adk/middlewares/skill"

	fsmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
)

// Project roles coordinate tasks independently of the shared skill catalog.
const OrchestratorPersona = `你是 SourceAEC cad->ifc 项目的编排者，按需要协调规划、CAD 与 IFC 建模。
先明确目标、输入和交付物。复杂规划可记录为 plan.json 与 bim_supplement.json，调用 deliver_plan 由 Go 保存。
需要 CAD 时派 cad-agent，需要 IFC 时派 ifc-agent；子任务应携带需求、projectID、modelId 和现有输入。
CAD 到 IFC 的输入可通过 stage_upstream_to_workdir 获取；不要臆造旧 CLI 或 SDK。
所有模型修改走 get_script → stage_script（baseRevision）→ run_script → save_script。
按实际运行结果报告版本、检查证据和未验证项。设计规范问答可以直接回答。`

const personaCAD = `你是 SourceAEC CAD 项目的协调者。按需要记录声明性规划输入，调用 deliver_plan 保存。
已有规划可通过 stage_plan_to_workdir 读取，派 cad-agent 在现有 modelId 上增量修改。
需要新图纸时使用 init_model；交付 building.json 用 deliver_building。只按实际检查结果报告。`
const personaIFC = `你是 SourceAEC IFC 项目的协调者。先核对设计意图，再派 ifc-agent 深化项目已有骨架。
复杂设计可以用声明性 JSON 记录需求；没有强制设计 DSL 或 Python CLI 前置。
模型修改使用脚本 API，检查实际 IFC 后保存。按验证结果报告，不臆造合规结论。`
const (
	PersonaIFC      = "ifc-agent"
	PersonaCAD      = "cad-agent"
	ifcAgentPersona = `你是 IFC 建模 Agent。读取已有 get_script，使用外部 ifcopenshell 库编写运行时模型源码。
顶层 PARAMS 使用字面量字典。程序接收输出路径作为第一个参数；使用已配置的外部几何库。
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
// The configured catalog is shared by every role; external SDK references may be installed alongside it.
func newSkillMiddleware(ctx context.Context, skillsDir string) (adk.TypedChatModelAgentMiddleware[*schema.Message], error) {
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
	skillMW, err := skill.NewTyped[*schema.Message](ctx, &skill.TypedConfig[*schema.Message]{
		Backend: skillBackend,
	})
	if err != nil {
		return nil, fmt.Errorf("create skill middleware: %w", err)
	}
	return skillMW, nil
}

// newFilesystemMiddleware exposes configured references and the bounded project
// workspace. Host commands remain subject to the configured allowlist.
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
	maxStep     int
	dataDir     string // 平台数据根（{DATA}）；filesystem Write 白名单根 = {DATA}/skill-work
}

// newRoleAgent 构造一个角色子 agent：独立 ChatModelAgent（Name/Description/
// Instruction/独立模型实例）+ 领域工具 + 角色 skill middleware + filesystem middleware
// + 工具错误兜底。不含 AgentAsTool——深度预算 1 结构性保证（子 agent 无派发工具）。
func newRoleAgent(ctx context.Context, cfg roleAgentConfig) (adk.Agent, error) {
	var handlers []adk.TypedChatModelAgentMiddleware[*schema.Message]
	if cfg.skillsDir != "" {
		skillMW, err := newSkillMiddleware(ctx, cfg.skillsDir)
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
