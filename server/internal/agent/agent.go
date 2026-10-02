// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// agent.go：Agent 装配核心——默认常量（persona/步数/上下文预算）、Agent 结构、
// New（ADK 三角色装配入口：orchestrator + ifc/cad 子 agent + middleware 链）与
// Persona 观察点。Option 配置域见 agent_options.go；运行域（Run/Resume/
// CheckPointStore）见 agent_run.go；三角色细节见 agents.go。
package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const defaultPersona = `你是 AI_IFC 平台的内置智能体，帮助设计师通过对话完成 IFC/CAD 模型的生成与修改。

编辑纪律（script-as-source：脚本是模型的唯一事实源，改模型 = 改脚本）：
- 先 get_script 读当前脚本，在既有脚本上做增量修改，禁止整体重写。
- 变更走 stage_script → run_script（沙箱验证）→ save_script（落大版本）三段式；run 失败先读错误改脚本再重试。
- 保持 PARAMS 的 key 稳定：只改值或新增 key，不改既有 key 名；设计意图优先用 PARAMS 参数化表达。
IFC 与 DXF 模型走同一套工具（后端按 kind 自动路由），每一步说明依据。`

const defaultMaxStep = 20

// DefaultMaxStep 是子 agent run 的默认步数上限（SubagentConfig.MaxStep 缺省值）。
const DefaultMaxStep = defaultMaxStep

// defaultAgentName 是主 agent 的默认名（模型 step/start 事件展示名；子 agent 经 WithName 覆盖）。
const defaultAgentName = "aiifc-main"

// defaultMaxContextChars 是模型 context 的字符近似（会话记忆阀门基准，historyBudgetRatio=60%）。
// 未超 60% 全量喂历史；超过触发语义压缩。1M 字符 ≈ 主流长上下文模型窗口
// （128K~1M token 量级；若部署模型窗口更小，用 WithMaxContextChars 调低）。
const defaultMaxContextChars = 1_000_000

type Agent struct {
	name            string
	persona         string // 最终生效的 Instruction（kind 变体或显式 WithPersona）
	runner          *adk.Runner
	store           *EventStore
	maxStep         int
	maxContextChars int
}

// New 装配 ADK 三角色（路线 B，D10）：
//   - orchestrator（主 agent）：领域工具 + AgentAsTool(ifc/cad) + EmitInternalEvents；
//     skill middleware 全量挂载（aiplan 对话协调层 + aibim-orchestrator 编排手册，D11）
//   - ifc-agent / cad-agent：独立 ChatModelAgent（各自 persona/独立模型实例/领域工具 +
//     skill middleware），被 AgentAsTool 包装进 orchestrator 工具面
//
// cfg.APIKey 为空（且未注入 WithModel）时回退确定性 scriptedModel，离线 demo 与测试
// 不依赖真模型。skillsDir 非空时挂官方 skill middleware（skill 工具自动进入模型工具面）。
func New(cfg LLMConfig, opts ...Option) (*Agent, error) {
	o := agentOptions{name: defaultAgentName, persona: defaultPersona, maxStep: defaultMaxStep, maxContextChars: defaultMaxContextChars}
	for _, opt := range opts {
		opt(&o)
	}
	cm := o.model
	if cm == nil {
		var err error
		cm, err = NewChatModel(context.Background(), cfg)
		if err != nil {
			return nil, fmt.Errorf("create chat model: %w", err)
		}
		if cm == nil {
			cm = defaultScriptedModel()
		}
	}
	childModel := o.childModel
	if childModel == nil {
		childModel = func() model.ToolCallingChatModel {
			c, err := NewChatModel(context.Background(), cfg)
			if err != nil || c == nil {
				return defaultScriptedModel()
			}
			return c
		}
	}
	ctx := context.Background()

	// 子 agent（ifc/cad）：独立模型实例 + 领域工具（A2 工具面按角色分离精化）
	// 角色 skill 映射（第一层：意图路由）：ifc-agent→aiifc、cad-agent→aidxf
	// ask_user 工具（HITL 开放断点）：orchestrator + 子 agent 都能问用户
	// **init_model 角色分离（2026-08-23）**：ifc 骨架在 create_project 已绑定（1 项目 = 1 ifc
	// 模型，项目创建即分配 modelId + 绑 projectId）——ifc-agent 禁止 init_model（只能在绑定模型
	// 上深化 stage/run/save）；cad-agent 保留（cad 项目空白，按需 init_model 建 DXF）。
	domainAndAsk := append(append([]tool.BaseTool{}, o.tools...), AskUserTool())
	ifcTools := FilterRoleTools(domainAndAsk, "init_model")
	ifcAgent, err := newRoleAgent(ctx, roleAgentConfig{
		name: PersonaIFC, description: "IFC 建模子 agent（aiifc skill，script-as-source：stage→run→save 三段式）",
		instruction: ifcAgentPersona, model: childModel(), tools: ifcTools,
		skillsDir: o.skillsDir, skills: []string{"aiifc"}, maxStep: o.maxStep, dataDir: o.dataDir,
	})
	if err != nil {
		return nil, err
	}
	cadAgent, err := newRoleAgent(ctx, roleAgentConfig{
		name: PersonaCAD, description: "CAD 绘图子 agent（aidxf skill，DXF 生成/校验；建筑平面任务对齐 plan 需求）",
		instruction: cadAgentPersona, model: childModel(), tools: domainAndAsk,
		skillsDir: o.skillsDir, skills: []string{"aidxf"}, maxStep: o.maxStep, dataDir: o.dataDir,
	})
	if err != nil {
		return nil, err
	}

	// orchestrator：领域工具 + AgentAsTool(按 kind 选择性装配) + EmitInternalEvents（子事件实时上浮）
	// 角色 skill 映射：cad/cad->ifc 管线 orchestrator→aiplan（对话协调层内联，D11）；
	// ifc 管线不挂 aiplan（无 plan 阶段）。编排手册按 kind 选 persona。
	var handlers []adk.TypedChatModelAgentMiddleware[*schema.Message]
	if o.skillsDir != "" && o.kind != "ifc" {
		skillMW, err := newSkillMiddleware(ctx, o.skillsDir, "aiplan")
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, skillMW)
	}
	// filesystem middleware（D12/M2-0）：读 skill references + execute 白名单（orchestrator 也需读 aiplan references）
	fsMW, err := newFilesystemMiddleware(ctx, skillWorkRootFor(o.dataDir), o.skillsDir)
	if err != nil {
		return nil, err
	}
	handlers = append(handlers, fsMW)
	// 工具错误兜底（官方 SafeToolMiddleware 形状）：工具 Go error → 文本结果
	// （"[tool error] ..."），翻译层恢复为带 error 载荷的 tool/result（单卡错误态），
	// 模型可见可自愈；interrupt 错误透传（HITL 原语不被吞）。
	handlers = append(handlers, newSafeToolMiddleware())
	// D3c：交付审批 middleware（调了先问）——拦截 save_script/deliver_plan，
	// 首次调用中断提问，用户经 /answer 确认后放行（官方 approval_wrapper 形态）。
	handlers = append(handlers, newApprovalMiddleware())

	persona := o.persona
	switch o.kind {
	case "cad":
		persona = personaCAD
	case "ifc":
		persona = personaIFC
	case "cad->ifc":
		persona = OrchestratorPersona // cad->ifc 专属全链编排（kind 强制三选一，无空 kind 兜底）
	}
	ag, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          o.name,
		Description:   "AI_IFC 平台主智能体：意图路由 + 领域工具（REST 沙箱交付）+ AgentAsTool(按 kind) + skill 技能包",
		Instruction:   persona,
		Model:         cm,
		MaxIterations: o.maxStep,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: orchestratorTools(domainAndAsk, kindChildren(o.kind, cadAgent, ifcAgent)...),
			},
			EmitInternalEvents: true, // 子 AgentEvent 实时上浮（翻译层 RunPath 打标）
		},
		Handlers: handlers,
	})
	if err != nil {
		return nil, fmt.Errorf("create adk chat model agent: %w", err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           ag,
		EnableStreaming: true,
		CheckPointStore: newMemoryCheckPointStore(), // HITL 前置：中断状态落检查点，Resume 续跑
	})
	return &Agent{name: o.name, persona: persona, runner: runner, store: o.store, maxStep: o.maxStep, maxContextChars: o.maxContextChars}, nil
}

// Persona 返回最终生效的 Instruction（kind 变体或显式 WithPersona）——路由/测试观察点。
func (a *Agent) Persona() string {
	return a.persona
}
