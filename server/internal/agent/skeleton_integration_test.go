package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestSkeletonRealSkillsIntegration：真实仓库 skills 目录 + scriptedModel 的
// 端到端骨架验证——共享 sourceaec 接入说明，调用返回真实 sourceaec SKILL.md 正文
// （progressive disclosure 内容片段）。验证「接入 ADK skill」在真实数据上闭环。
func TestSkeletonRealSkillsIntegration(t *testing.T) {
	if _, err := os.Stat(maintainedSkillsDir()); err != nil {
		t.Fatalf("maintained skill directory missing: %v", err)
	}
	ag, err := New(LLMConfig{},
		WithSkillsDir(maintainedSkillsDir()),
		WithModel(NewScriptedModel(Script{Steps: []ScriptStep{
			{ToolCalls: []ToolCallSpec{{ID: "s1", Name: "skill", Arguments: `{"skill":"sourceaec"}`}}},
			{Chunks: []string{"已加载 sourceaec skill"}},
		}})),
		WithMaxStep(10),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := ag.Run(context.Background(), "sess-real-skill", "加载 sourceaec")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	evs := collect(t, ch)

	var content string
	found := false
	for _, ev := range evs {
		if ev.Type != EventToolResult {
			continue
		}
		if payloadString(t, ev, "name") != "skill" {
			continue
		}
		found = true
		content = payloadString(t, ev, "content")
	}
	if !found {
		t.Fatalf("未观测到 skill 工具结果；types=%v", eventTypes(evs))
	}
	// 真实 sourceaec SKILL.md 的关键片段（官方 defaultSkillContent 组装：
	// "Launching skill: <name>" + BaseDirectory + SKILL.md 正文）
	for _, want := range []string{"Launching skill: sourceaec", "SourceAEC integration"} {
		if !strings.Contains(content, want) {
			t.Errorf("skill 结果缺 %q 片段；content=%s...", want, truncate(content, 120))
		}
	}
	if !strings.Contains(content, "/skills/sourceaec") {
		t.Errorf("skill 结果缺 BaseDirectory 路径（/skills/sourceaec）")
	}
	// 整轮正常收尾
	if last := evs[len(evs)-1]; last.Type != EventTurnEnd {
		t.Fatalf("末事件 = %s, want turn/end", last.Type)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
