// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_flows_contract_test.go：W-0051 三类对话全链 SSE 帧序列专项回归（前两类：
// plan→cad→ifc / cad 带计划；ifc 独立与失败路径见 chat_flows_ifc_failure_test.go）。
// scriptedModel 驱动真实 REST + SSE 流，逐帧钉：
//   - 事件 kind 序列（session.status / message.* / subagent.status / question.ask）
//   - 子事件标签 sa_{turn}_{seq} 边界 + subagent.status started/finished 配对
//   - 交付产物落盘（PlanStore 版本化 + models/{modelId}/ 注册 + skill 工作区桥接）
//
// 夹具与断言 helper 见 chat_flows_helpers_test.go。
package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ifcviewer/server/internal/agent"
	"ifcviewer/server/internal/store"
)

// --- ① plan→cad→ifc 全链（kind cad->ifc） ---

// TestFlowPlanCadIFCFullChain：plan 交付（审批中断→确认）→ cad 消费计划（stage
// plan→init_model→deliver_building）→ ifc 消化上游（stage upstream→run_script）→
// 主汇总。逐帧钉全链 SSE 序列 + sa_1_1/sa_1_2 子代理边界 + 产物落盘。
func TestFlowPlanCadIFCFullChain(t *testing.T) {
	main := agent.Script{Steps: []agent.ScriptStep{
		{ToolCalls: []agent.ToolCallSpec{{ID: "d1", Name: "deliver_plan",
			Arguments: `{"plan":{"version":2,"zones":[]},"bimSupplement":{"roof":"flat"}}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "c1", Name: agent.PersonaCAD, Arguments: `{"request":"按 plan 出图"}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "i1", Name: agent.PersonaIFC, Arguments: `{"request":"消费上游深化 IFC"}`}}},
		{Chunks: []string{"全链完成"}},
	}}
	cadChild := agent.Script{Steps: []agent.ScriptStep{
		{ToolCalls: []agent.ToolCallSpec{{ID: "s1", Name: "stage_plan_to_workdir", Arguments: `{}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "m1", Name: "init_model", Arguments: `{"kind":"dxf","title":"一层平面"}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "b1", Name: "deliver_building",
			Arguments: `{"building":{"zones":[{"zone":"tower","modelId":"m_0123456789abcdef"}]}}`}}},
		{Chunks: []string{"CAD 出图完成"}},
	}}
	fw := newFlowHarness(t)

	// 建 cad->ifc 项目（REST）：ifc 骨架随建（= init_model(ifc) 绑定）
	r := doCreateProject(t, fw.h, `{"title":"全链项目","kind":"cad->ifc"}`)
	p, err := fw.ps.Get(r.ProjectID)
	if err != nil || len(p.Models) != 1 || p.Models[0].Kind != store.KindIFC {
		t.Fatalf("cad->ifc 项目应含 1 个 ifc 骨架: %v err=%v", p, err)
	}
	skelID := p.Models[0].ID
	waitConvertByModel(t, fw.runs, fw.h.deps.St, skelID)
	// ifc 子代理深化骨架：显式带 modelId（项目会话不绑模型——生产 agent 从上游
	// 锚点取 modelId 传参；scripted 脚本在项目创建后内插，确定性不变）
	ifcChild := agent.Script{Steps: []agent.ScriptStep{
		{ToolCalls: []agent.ToolCallSpec{{ID: "u1", Name: "stage_upstream_to_workdir", Arguments: `{}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "r1", Name: "run_script", Arguments: `{"modelId":"` + skelID + `"}`}}},
		{Chunks: []string{"IFC 深化完成"}},
	}}
	fw.attachKindAgent(t, "cad->ifc", main, &ifcChild, &cadChild)
	// 预置 zone DXF（building zones 引用的平台模型当前态，upstream 桥接复制源）
	uploads := filepath.Join(fw.h.deps.DataDir, "uploads")
	if err := os.MkdirAll(uploads, 0o755); err != nil {
		t.Fatal(err)
	}
	zoneDXF := []byte("0\nSECTION\n2\nENTITIES\n0\nENDSEC\n0\nEOF\n")
	if err := os.WriteFile(filepath.Join(uploads, "m_0123456789abcdef.dxf"), zoneDXF, 0o644); err != nil {
		t.Fatal(err)
	}
	// plan 注入（REST PUT → v1 归档；deliver_plan 后升 v2）
	flowReq(t, fw.h, http.MethodPut, "/api/v1/projects/"+r.ProjectID+"/plan.json",
		`{"content":{"version":1,"site":{"area":100}}}`)
	flowReq(t, fw.h, http.MethodPut, "/api/v1/projects/"+r.ProjectID+"/bim_supplement.json",
		`{"content":{"version":1}}`)
	// 会话只绑项目（与生产 web 装配一致；run_script 显式带 modelId）——项目会话
	// 无模型绑定 → idle 后 notify 管线不触发（cs.ModelID 为空），帧序干净
	cs := doChatCreateJSON(t, fw.h, `{"title":"t","projectId":"`+r.ProjectID+`"}`)
	ch := fw.h.subscribe(cs.ID)

	if code := postChat(t, fw.h, cs.ID, "从方案到 IFC 全链"); code != http.StatusOK {
		t.Fatalf("post status = %d", code)
	}
	// post 阶段：deliver_plan 审批中断（调了先问），turn 悬停不 idle
	post := collectUntil(t, ch, "question.ask")
	assertEventSeq(t, post, []string{
		"session.status", "message.updated", // turn/start：busy + user 消息
		"message.updated", "message.part.updated", // 模型步：assistant 行 + deliver_plan 卡 running
		"question.ask", // 审批中断（无 turn/end → 不 idle）
	})
	rest := flowAnswer(t, fw.h, ch, cs.ID, post, "确认")
	assertEventSeq(t, rest, []string{
		"message.part.updated",                    // deliver_plan 卡 completed（确认放行）
		"message.updated", "message.part.updated", // 模型步：assistant 行 + cad-agent 派发卡 running
		"subagent.status",                              // sa_1_1 started（cad-agent）
		"message.part.updated", "message.part.updated", // 子：stage_plan_to_workdir running/completed
		"message.part.updated", "message.part.updated", // 子：init_model running/completed（model.created 另钉）
		"message.part.updated", "message.part.updated", // 子：deliver_building running/completed
		"message.part.updated", "message.part.delta", // 子最终答复（分片）
		"subagent.status",                         // sa_1_1 finished
		"message.part.updated",                    // cad-agent 派发卡 completed（output=子最终答复）
		"message.updated", "message.part.updated", // 模型步：assistant 行 + ifc-agent 派发卡 running
		"subagent.status",                              // sa_1_2 started（ifc-agent）
		"message.part.updated", "message.part.updated", // 子：stage_upstream_to_workdir running/completed
		"message.part.updated", "message.part.updated", // 子：run_script running/completed（viewer.staged 另钉）
		"message.part.updated", "message.part.delta", // 子最终答复（分片）
		"subagent.status",                                               // sa_1_2 finished
		"message.part.updated",                                          // ifc-agent 派发卡 completed
		"message.updated", "message.part.updated", "message.part.delta", // 主汇总文本（分片）
		"session.status", "session.idle", // turn 收尾
	})

	// 子代理边界：sa_1_1(cad) → sa_1_2(ifc)，started/finished 严格配对且人格正确
	subs := flowSubStatuses(t, rest)
	if len(subs) != 4 ||
		subs[0].id != "sa_1_1" || subs[0].status != "started" || subs[0].persona != agent.PersonaCAD ||
		subs[1].id != "sa_1_1" || subs[1].status != "finished" ||
		subs[2].id != "sa_1_2" || subs[2].status != "started" || subs[2].persona != agent.PersonaIFC ||
		subs[3].id != "sa_1_2" || subs[3].status != "finished" {
		t.Fatalf("subagent.status 配对不符: %+v", subs)
	}
	if !strings.Contains(subs[0].task, "按 plan 出图") || !strings.Contains(subs[2].task, "消费上游") {
		t.Fatalf("subagent.status task 应携带父派发 request: %+v", subs)
	}
	// started 先于子 part、finished 后于子 part 且先于父派发卡 completed（边界契约）
	firstChild := flowIndexOf(rest, "message.part.updated", `"subagentId":"sa_1_1"`)
	finished1 := flowIndexOf(rest, "subagent.status", `"status":"finished"`)
	cadDone := flowIndexOf(rest, "message.part.updated", `"status":"completed","title":"cad-agent"`)
	if !(flowIndexOf(rest, "subagent.status", `"status":"started"`) < firstChild && firstChild < finished1 && finished1 < cadDone) {
		t.Fatalf("sa_1_1 边界顺序不符：started/child/finished/父卡 = %d/%d/%d/%d",
			flowIndexOf(rest, "subagent.status", `"status":"started"`), firstChild, finished1, cadDone)
	}
	// 已完成工具卡序（tool, subagentId）：计划交付 → cad 子三工具 → 派发汇总 → ifc 子两工具 → 派发汇总
	assertCompletedCards(t, rest, []flowCard{
		{tool: "deliver_plan"},
		{tool: "stage_plan_to_workdir", subID: "sa_1_1"},
		{tool: "init_model", subID: "sa_1_1"},
		{tool: "deliver_building", subID: "sa_1_1"},
		{tool: agent.PersonaCAD},
		{tool: "stage_upstream_to_workdir", subID: "sa_1_2"},
		{tool: "run_script", subID: "sa_1_2"},
		{tool: agent.PersonaIFC},
	})
	// 派发卡 output = 子最终答复（汇总纪律：原样转述）
	for _, c := range flowToolCards(t, rest) {
		if c.tool == agent.PersonaCAD && c.status == "completed" && !strings.Contains(c.output, "CAD 出图完成") {
			t.Fatalf("cad-agent 派发卡应转述子最终答复: %+v", c)
		}
		if c.tool == agent.PersonaIFC && c.status == "completed" && !strings.Contains(c.output, "IFC 深化完成") {
			t.Fatalf("ifc-agent 派发卡应转述子最终答复: %+v", c)
		}
	}
	// viewer.staged：骨架模型 + kind ifc，主会话帧不带 subagentId，先于 run_script
	// completed 卡（工具内直推 → 必先于其 completed 帧落线）
	runDone := flowIndexOf(rest, "message.part.updated", `"status":"completed","title":"run_script"`)
	assertInject(t, rest, "viewer.staged", map[string]any{"modelId": skelID, "kind": store.KindIFC}, runDone)
	if _, has := flowFrameData(t, rest[flowIndexOf(rest, "viewer.staged", "")])["subagentId"]; has {
		t.Fatal("主会话系统帧不得带 subagentId")
	}
	// model.created：cad 新 DXF 模型注册（kind dxf），先于 init_model completed 卡
	initDone := flowIndexOf(rest, "message.part.updated", `"status":"completed","title":"init_model"`)
	assertInject(t, rest, "model.created", map[string]any{"kind": store.KindDXF}, initDone)
	created := flowFrameData(t, rest[flowIndexOf(rest, "model.created", "")])
	newDXF := created["modelId"].(string)
	if newDXF == "" {
		t.Fatalf("model.created 缺 modelId: %v", created)
	}

	// --- 产物落盘 ---
	// plan 版本化：deliver 后 current=v2 内容、v1 归档
	cur, err := fw.planSt.Get(r.ProjectID, "plan.json")
	if err != nil || !strings.Contains(string(cur), `"version":2`) {
		t.Fatalf("deliver_plan 后 plan 应为 v2: %s err=%v", cur, err)
	}
	hist, _ := fw.planSt.ListHistory(r.ProjectID, "plan.json")
	if len(hist) != 1 || hist[0] != "v1" {
		t.Fatalf("plan history = %v, want [v1]", hist)
	}
	// building 交付（zones 记 modelId）
	bld, err := fw.planSt.Get(r.ProjectID, "building.json")
	if err != nil || !strings.Contains(string(bld), "m_0123456789abcdef") {
		t.Fatalf("building.json 未交付或 zones 缺 modelId: %s err=%v", bld, err)
	}
	// cad 新模型注册：models/{id}/ 目录 + 项目聚合（骨架 + 新 DXF = 2）
	if fi, err := os.Stat(filepath.Join(fw.h.deps.DataDir, "models", newDXF)); err != nil || !fi.IsDir() {
		t.Fatalf("models/%s/ 未注册: %v", newDXF, err)
	}
	p2, _ := fw.ps.Get(r.ProjectID)
	if len(p2.Models) != 2 {
		t.Fatalf("项目模型数 = %d, want 2（骨架 ifc + cad 新 dxf）", len(p2.Models))
	}
	// ifc 消费上游桥接落盘：skill-work/{pid}/building.json + bim + dxf/tower.dxf
	work := filepath.Join(fw.h.deps.DataDir, "skill-work", r.ProjectID)
	for _, rel := range []string{"building.json", "bim_supplement.json", filepath.Join("dxf", "tower.dxf")} {
		if _, err := os.Stat(filepath.Join(work, rel)); err != nil {
			t.Fatalf("上游桥接产物 %s 未落盘: %v", rel, err)
		}
	}
	// kind 路由：骨架/深化走 ifc 后端、cad 新模型走 cad 后端（零交叉）
	if fw.ifcEd.countContaining("/models/"+skelID+"/script/run") < 2 {
		t.Fatal("ifc 后端应收到骨架 init run + 子代理 run_script")
	}
	if fw.cadEd.countContaining("/models/"+newDXF+"/script") < 1 {
		t.Fatal("cad 后端应收到新 dxf 模型的骨架 stage/run/save")
	}
}

// --- ② cad 带计划（kind cad） ---

// TestFlowCadWithPlanDeliversBuilding：plan 注入（REST PUT）→ cad 子代理消费计划
// （get_project_plans → stage_plan_to_workdir → deliver_building）→ 主汇总。
// 单阶段直达 idle（deliver_building 非审批工具），钉帧序列 + sa_1_1 边界。
func TestFlowCadWithPlanDeliversBuilding(t *testing.T) {
	main := agent.Script{Steps: []agent.ScriptStep{
		{ToolCalls: []agent.ToolCallSpec{{ID: "c1", Name: agent.PersonaCAD, Arguments: `{"request":"按注入的 plan 出图"}`}}},
		{Chunks: []string{"CAD 交付完成"}},
	}}
	cadChild := agent.Script{Steps: []agent.ScriptStep{
		{ToolCalls: []agent.ToolCallSpec{{ID: "g1", Name: "get_project_plans", Arguments: `{}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "s1", Name: "stage_plan_to_workdir", Arguments: `{}`}}},
		{ToolCalls: []agent.ToolCallSpec{{ID: "b1", Name: "deliver_building",
			Arguments: `{"building":{"zones":[{"zone":"tower","modelId":"m_0123456789abcdef"}]}}`}}},
		{Chunks: []string{"CAD 出图完成"}},
	}}
	fw := newFlowHarness(t)
	fw.attachKindAgent(t, "cad", main, nil, &cadChild)

	r := doCreateProject(t, fw.h, `{"title":"带计划 CAD","kind":"cad"}`)
	flowReq(t, fw.h, http.MethodPut, "/api/v1/projects/"+r.ProjectID+"/plan.json",
		`{"content":{"version":1,"project":"`+r.ProjectID+`","zones":[{"zone":"tower"}]}}`)
	flowReq(t, fw.h, http.MethodPut, "/api/v1/projects/"+r.ProjectID+"/bim_supplement.json",
		`{"content":{"version":1,"roof":"flat"}}`)
	// 项目会话无模型绑定（cad 项目空白）→ idle 后无 notify 管线
	cs := doChatCreateJSON(t, fw.h, `{"title":"t","projectId":"`+r.ProjectID+`"}`)
	ch := fw.h.subscribe(cs.ID)

	if code := postChat(t, fw.h, cs.ID, "带计划出图"); code != http.StatusOK {
		t.Fatalf("post status = %d", code)
	}
	frames := collectUntil(t, ch, "session.idle")
	assertEventSeq(t, frames, []string{
		"session.status", "message.updated", // turn/start：busy + user 消息
		"message.updated", "message.part.updated", // 模型步：assistant 行 + cad-agent 派发卡 running
		"subagent.status",                              // sa_1_1 started
		"message.part.updated", "message.part.updated", // 子：get_project_plans running/completed
		"message.part.updated", "message.part.updated", // 子：stage_plan_to_workdir running/completed
		"message.part.updated", "message.part.updated", // 子：deliver_building running/completed
		"message.part.updated", "message.part.delta", // 子最终答复（分片）
		"subagent.status",                                               // sa_1_1 finished
		"message.part.updated",                                          // cad-agent 派发卡 completed
		"message.updated", "message.part.updated", "message.part.delta", // 主汇总文本（分片）
		"session.status", "session.idle",
	})
	// sa_1_1 started/finished 配对（单一子代理窗口）
	subs := flowSubStatuses(t, frames)
	if len(subs) != 2 || subs[0].id != "sa_1_1" || subs[0].status != "started" ||
		subs[1].id != "sa_1_1" || subs[1].status != "finished" || subs[0].persona != agent.PersonaCAD {
		t.Fatalf("subagent.status 配对不符: %+v", subs)
	}
	assertCompletedCards(t, frames, []flowCard{
		{tool: "get_project_plans", subID: "sa_1_1"},
		{tool: "stage_plan_to_workdir", subID: "sa_1_1"},
		{tool: "deliver_building", subID: "sa_1_1"},
		{tool: agent.PersonaCAD},
	})
	// 注入的 plan 可读（v1）+ building 交付（版本化）
	cur, err := fw.planSt.Get(r.ProjectID, "plan.json")
	if err != nil || !strings.Contains(string(cur), `"tower"`) {
		t.Fatalf("注入 plan 未生效: %s err=%v", cur, err)
	}
	bld, err := fw.planSt.Get(r.ProjectID, "building.json")
	if err != nil || !strings.Contains(string(bld), "m_0123456789abcdef") {
		t.Fatalf("building.json 未交付: %s err=%v", bld, err)
	}
	// 全链无模型创建（cad 子未 init_model）→ 项目保持空白
	p, _ := fw.ps.Get(r.ProjectID)
	if len(p.Models) != 0 {
		t.Fatalf("本项目不应注册模型: %+v", p.Models)
	}
}
