// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_flows_ifc_failure_test.go：W-0051 全链 SSE 帧序列专项回归（后两类：
// ifc 独立管线 / 工具失败路径）。夹具与断言 helper 见 chat_flows_contract_test.go。
package api

import (
	"net/http"
	"strings"
	"testing"

	"ifcviewer/server/internal/agent"
	"ifcviewer/server/internal/store"
)

// --- ③ ifc 独立（kind ifc） ---

// TestFlowIFCIndependentScriptChain：ifc 项目（建项目即 init_model(ifc) 绑定骨架）
// → 主 agent 直连工具面 get_script → stage_script → run_script（中途预览）→
// save_script（审批中断→确认放行）→ 汇总。钉主会话工具卡帧序 + 审批断点 + 下一轮可续。
func TestFlowIFCIndependentScriptChain(t *testing.T) {
	fw := newFlowHarness(t)
	// ifc 项目：建项目即初始化骨架（= init_model(ifc)，modelId 绑定项目）
	r := doCreateProject(t, fw.h, `{"title":"独立 IFC","kind":"ifc"}`)
	p, err := fw.ps.Get(r.ProjectID)
	if err != nil || len(p.Models) != 1 || p.Models[0].Kind != store.KindIFC {
		t.Fatalf("ifc 项目应含 1 个骨架: %+v err=%v", p, err)
	}
	skelID := p.Models[0].ID
	waitConvertByModel(t, fw.runs, fw.h.deps.St, skelID)
	// 主 agent 直连工具面（ifc 独立不派子代理）：骨架 modelId 显式传参
	// （项目会话不绑模型——生产 agent 从 get_project_models 取 modelId）
	main := agent.Script{Steps: []agent.ScriptStep{
		{ToolCalls: []agent.ToolCallSpec{{ID: "g1", Name: "get_script", Arguments: `{"modelId":"` + skelID + `"}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "s1", Name: "stage_script",
			Arguments: `{"modelId":"` + skelID + `","script":"PARAMS = {\"height\": 3600}","note":"层高"}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "r1", Name: "run_script", Arguments: `{"modelId":"` + skelID + `"}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "v1", Name: "save_script",
			Arguments: `{"modelId":"` + skelID + `","note":"深化 v2"}`}}},
		{Chunks: []string{"IFC 深化完成"}},
		{Chunks: []string{"下一轮正常"}},
	}}
	fw.attachKindAgent(t, "ifc", main, nil, nil)
	// 会话只绑项目（与生产 web 装配一致）——无模型绑定 → idle 后无 notify 管线
	cs := doChatCreateJSON(t, fw.h, `{"title":"t","projectId":"`+r.ProjectID+`"}`)
	ch := fw.h.subscribe(cs.ID)

	if code := postChat(t, fw.h, cs.ID, "深化这个建筑"); code != http.StatusOK {
		t.Fatalf("post status = %d", code)
	}
	// post 阶段：get→stage→run 逐卡推进，save_script 审批中断（调了先问）
	post := collectUntil(t, ch, "question.ask")
	assertEventSeq(t, post, []string{
		"session.status", "message.updated", // turn/start：busy + user 消息
		"message.updated", "message.part.updated", "message.part.updated", // get_script 行 + 卡 running/completed
		"message.updated", "message.part.updated", "message.part.updated", // stage_script（三段式之一）
		"message.updated", "message.part.updated", "message.part.updated", // run_script（viewer.staged 另钉）
		"message.updated", "message.part.updated", // save_script 行 + 卡 running
		"question.ask", // 审批中断（无 turn/end → 不 idle）
	})
	// viewer.staged：骨架模型 + kind ifc，先于 run_script completed 卡
	runDone := flowIndexOf(post, "message.part.updated", `"status":"completed","title":"run_script"`)
	assertInject(t, post, "viewer.staged", map[string]any{"modelId": skelID, "kind": store.KindIFC}, runDone)
	// ifc 独立不派子代理（主 agent 直连工具面）：全程无 subagent 帧
	if subs := flowSubStatuses(t, post); len(subs) != 0 {
		t.Fatalf("ifc 独立不应出现 subagent.status: %+v", subs)
	}

	rest := flowAnswer(t, fw.h, ch, cs.ID, post, "确认")
	assertEventSeq(t, rest, []string{
		"message.part.updated",                                          // save_script 卡 completed（确认放行 → 真执行）
		"message.updated", "message.part.updated", "message.part.delta", // 汇总文本（分片）
		"session.status", "session.idle", // turn 收尾
	})
	// save 真执行（审批放行）：骨架 v1 + 深化 v2 共两次落版本
	if n := fw.ifcEd.countContaining("POST /models/" + skelID + "/script/save"); n != 2 {
		t.Fatalf("script/save 调用数 = %d, want 2（骨架 init + 审批放行后的深化）", n)
	}
	// 会话不卡死：审批轮结束后下一轮照常收尾（scripted 脚本续位，正常文本轮）
	if code := postChat(t, fw.h, cs.ID, "再来一轮"); code != http.StatusOK {
		t.Fatalf("下一轮 post status = %d", code)
	}
	next := collectUntil(t, ch, "session.idle")
	assertEventSeq(t, next, []string{
		"session.status", "message.updated", // 新 turn：busy + user 消息
		"message.updated", "message.part.updated", "message.part.delta", // 正常文本产出
		"session.status", "session.idle",
	})
}

// --- ④ 失败路径全链：错误 kind 路由 + 文本化 + 64KB 截断 + 会话可续 ---

// TestFlowToolFailureObservableAndRecoverable：模型会话直连工具面——
//   - 工具参数类型错误 → 工具卡单卡错误态（status:"error" + error 文本），不刷
//     session.error 横幅、循环继续（模型可自愈）；
//   - 工具结果超 64KB → 文本化截断（"...(truncated)" 后缀可见）；
//   - 未知工具 → 会话级错误路由（session.error）+ 正常 idle 收尾；
//   - 失败后下一轮照常推进（会话不卡死）。
func TestFlowToolFailureObservableAndRecoverable(t *testing.T) {
	main := agent.Script{Steps: []agent.ScriptStep{
		{ToolCalls: []agent.ToolCallSpec{{ID: "bad1", Name: "stage_script", Arguments: `{"script":123}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "big1", Name: "get_script", Arguments: `{}`}}},
		{Chunks: []string{"已恢复"}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "nope1", Name: "rename_wall", Arguments: `{}`}}},
		{Chunks: []string{"下一轮正常"}},
	}}
	fw := newFlowHarness(t)
	fw.attachKindAgent(t, "", main, nil, nil)
	// get_script 返回超 64KB 正文（截断标记应在工具卡 output 可见）
	m, err := fw.h.deps.St.CreateWithKind("a.ifc", 4, strings.NewReader("fake"), store.KindIFC)
	if err != nil {
		t.Fatal(err)
	}
	fw.ifcEd.set(http.MethodGet, "/models/"+m.ID+"/script",
		`{"script":"`+strings.Repeat("a", 70000)+`"}`)
	cs := doChatCreateJSON(t, fw.h, `{"title":"t","modelId":"`+m.ID+`"}`)
	ch := fw.h.subscribe(cs.ID)

	// 轮 1：参数错误卡 + 截断卡 + 自愈汇总
	if code := postChat(t, fw.h, cs.ID, "改脚本"); code != http.StatusOK {
		t.Fatalf("post status = %d", code)
	}
	turn1 := collectUntil(t, ch, "session.idle")
	assertEventSeq(t, turn1, []string{
		"session.status", "message.updated", // turn/start：busy + user 消息
		"message.updated", "message.part.updated", "message.part.updated", // stage_script 行 + 卡 running/错误态
		"message.updated", "message.part.updated", "message.part.updated", // get_script 行 + 卡 running/completed
		"message.updated", "message.part.updated", "message.part.delta", // 模型自愈汇总
		"session.status", "session.idle",
	})
	for _, f := range turn1 {
		if strings.Contains(f, "event: session.error") {
			t.Fatalf("工具级失败不得刷 session.error 横幅:\n%s", strings.Join(turn1, "---\n"))
		}
	}
	// stage_script 单卡错误态：status=error + error 文本（kind 路由到卡，不整轮报错）
	var sawErrCard bool
	for _, c := range flowToolCards(t, turn1) {
		if c.tool == "stage_script" && c.status == "error" {
			sawErrCard = true
			if c.errText == "" {
				t.Fatalf("错误卡缺 error 文本: %+v", c)
			}
		}
	}
	if !sawErrCard {
		t.Fatal("stage_script 参数错误应产单卡错误态（status:error）")
	}
	// get_script 64KB 截断：completed 卡 output 带截断后缀（文本化上限可见）
	var sawTruncated bool
	for _, c := range flowToolCards(t, turn1) {
		if c.tool == "get_script" && c.status == "completed" {
			if !strings.HasSuffix(c.output, "...(truncated)") {
				t.Fatalf("超限工具结果应带截断后缀: len=%d 尾=%q", len(c.output), tail(c.output, 40))
			}
			if len(c.output) > 65536+len("...(truncated)") {
				t.Fatalf("截断后仍超上限: len=%d", len(c.output))
			}
			sawTruncated = true
		}
	}
	if !sawTruncated {
		t.Fatal("get_script 卡未见截断 output")
	}

	// 轮 2：未知工具（模型幻觉工具名）→ 会话级错误路由 + 正常 idle 收尾（不挂死）
	if code := postChat(t, fw.h, cs.ID, "试试乱调工具"); code != http.StatusOK {
		t.Fatalf("post status = %d", code)
	}
	turn2 := collectUntil(t, ch, "session.idle")
	assertEventSeq(t, turn2, []string{
		"session.status", "message.updated", // turn/start
		"message.updated", "message.part.updated", // 模型步：assistant 行 + 幻觉工具卡 running
		"session.error",                  // 未知工具：错误浮到会话级（翻译层 error → session.error）
		"session.status", "session.idle", // 收尾不缺：idle 照常（会话不卡死）
	})

	// 轮 3：会话可续——scripted 脚本续位，正常文本轮收尾
	if code := postChat(t, fw.h, cs.ID, "还能继续吗"); code != http.StatusOK {
		t.Fatalf("post status = %d", code)
	}
	turn3 := collectUntil(t, ch, "session.idle")
	assertEventSeq(t, turn3, []string{
		"session.status", "message.updated", // 新 turn
		"message.updated", "message.part.updated", "message.part.delta", // 正常文本产出
		"session.status", "session.idle",
	})
}

// tail 取字符串尾部 n 字节（失败信息展示用）。
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
