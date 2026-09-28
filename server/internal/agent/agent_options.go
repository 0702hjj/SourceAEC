// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// agent_options.go：Agent 的 Option 配置域（从 agent.go 拆出，W-0057 行数门控）——
// agentOptions 装配参数 + With* 函数式选项（名/模型/工具/persona/步数/上下文预算/
// 事件存储/skill 目录/数据根/kind 选择性装配）。
package agent

import (
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
)

type Option func(*agentOptions)

type agentOptions struct {
	name            string
	kind            string // 项目类型 cad/ifc/cad->ifc：决定 AgentAsTool 选择性装配 + persona + aiplan skill
	model           model.ToolCallingChatModel
	childModel      func() model.ToolCallingChatModel // 子 agent 模型工厂（路线 B；nil 时默认新建）
	tools           []tool.BaseTool
	persona         string
	maxStep         int
	maxContextChars int // 模型 context 字符近似（会话记忆阀门基准）
	store           *EventStore
	skillsDir       string // 扁平 skills 目录（BaseDir/*/SKILL.md）；空 = 不挂 skill middleware
	dataDir         string // 平台数据根（{DATA}）；filesystem Write/Edit 白名单根 = {DATA}/skill-work
}

// WithName 设置 agent 名（step/start 事件展示名，供前端区分主/子角色）。
func WithName(name string) Option {
	return func(o *agentOptions) { o.name = name }
}

func WithModel(m model.ToolCallingChatModel) Option {
	return func(o *agentOptions) { o.model = m }
}

// WithChildModelFactory 设置子 agent（ifc/cad）的模型工厂（路线 B）：
// 每次调用产出一个独立实例（scriptedModel 有 pos 游标，主/子必须独立；
// openai 无状态可共享但独立更干净）。nil 时默认按 cfg 新建、空回退 scripted。
func WithChildModelFactory(f func() model.ToolCallingChatModel) Option {
	return func(o *agentOptions) { o.childModel = f }
}

func WithTools(tools []tool.BaseTool) Option {
	return func(o *agentOptions) { o.tools = tools }
}

func WithPersona(persona string) Option {
	return func(o *agentOptions) { o.persona = persona }
}

func WithMaxStep(n int) Option {
	return func(o *agentOptions) { o.maxStep = n }
}

// WithMaxContextChars 设置模型 context 的字符近似（会话记忆阀门基准）：
// 历史未超 60% 全量喂，超预算语义压缩。默认 defaultMaxContextChars。
func WithMaxContextChars(n int) Option {
	return func(o *agentOptions) { o.maxContextChars = n }
}

func WithStore(s *EventStore) Option {
	return func(o *agentOptions) { o.store = s }
}

// WithSkillsDir 挂载官方 skill middleware（adk/middlewares/skill）：
// 目录须为扁平结构（BaseDir/*/SKILL.md），middleware 自动注入 skill 工具
// 与 progressive disclosure 系统提示词。空目录/不传 = 不挂载（离线/测试路径）。
func WithSkillsDir(dir string) Option {
	return func(o *agentOptions) { o.skillsDir = dir }
}

// WithDataDir 设置平台数据根（{DATA}）——filesystem Write/Edit 白名单根 = {DATA}/skill-work
// （agent 可在 skill 工作区写 design.json 等 LLM 意图中间产物；其它路径拒绝）。
func WithDataDir(dir string) Option {
	return func(o *agentOptions) { o.dataDir = dir }
}

// WithKind 按项目类型（cad/ifc/cad->ifc）选择性装配 orchestrator：
//   - AgentAsTool：cad->ifc 全装 cad+ifc；cad 只装 cad；ifc 只装 ifc
//   - persona：cad→OrchestratorPersonaCAD、ifc→OrchestratorPersonaIFC、其余默认
//   - aiplan skill：cad/cad->ifc 挂 orchestrator；ifc 管线不挂（无 plan 阶段）
// cad 管线语义 = aiplan → cad-agent 设计完即止（不派 ifc，等价「只走到 CAD
// 设计完」的 orches 版本）；cad->ifc = 全链（plan → cad → ifc）。
// 空值 = 全装（向后兼容：ifc+cad + aiplan）。
func WithKind(kind string) Option {
	return func(o *agentOptions) { o.kind = kind }
}
