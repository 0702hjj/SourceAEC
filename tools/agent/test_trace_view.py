#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""test_trace_view.py —— 产物提取 / 轨迹行提取与渲染 纯函数单测。"""
import trace_view
from trace_view import extract_artifacts, extract_trace, render_trace, trace_line


def _frame(seq, event, data, ts="2026-09-18T12:00:0%d.000"):
    return {"seq": seq, "ts": ts % (seq % 10), "event": event, "sid": seq, "data": data}


# ---- 产物提取 ----------------------------------------------------------------

def test_extract_artifacts_model_id_and_paths():
    blob = '模型 m_0123456789abcdef 已创建，脚本 data/projects/p1/scripts/v1.py'
    assert extract_artifacts(blob) == ["m_0123456789abcdef", "data/projects/p1/scripts/v1.py"]


def test_extract_artifacts_dedup_and_order():
    blob = "先 plan.json 再 m_ffffffffffffffff，又回 plan.json"
    arts = extract_artifacts(blob)
    assert arts == ["m_ffffffffffffffff", "plan.json"]


def test_extract_artifacts_strips_trailing_punct():
    assert extract_artifacts('out: skill-work/w-1/building.json,') == ["skill-work/w-1/building.json"]


def test_extract_artifacts_empty():
    assert extract_artifacts("") == []
    assert extract_artifacts(None) == []
    assert extract_artifacts("没有产物的普通文本") == []


def test_extract_artifacts_model_id_shape_strict():
    # 15 位/17 位 hex 不匹配（防把普通词当 modelId）
    assert extract_artifacts("m_0123456789abcde m_0123456789abcdef0") == []


# ---- 轨迹行提取 --------------------------------------------------------------

def _records():
    return [
        _frame(1, "message.updated", {"info": {"role": "user"}}),
        _frame(2, "subagent.status", {"subagentId": "sa_1_1", "persona": "aidxf",
                                      "status": "started", "task": "生成图纸"}),
        _frame(3, "message.part.updated", {
            "subagentId": "sa_1_1",
            "part": {"type": "tool", "tool": "execute", "state": {
                "status": "completed", "title": "execute",
                "output": "产物 skill-work/w-0001/building.json + plan.json"}}}),
        _frame(4, "message.part.updated", {
            "part": {"type": "tool", "tool": "render_model", "state": {
                "status": "completed", "title": "render_model",
                "output": "渲染完成 m_0123456789abcdef"}}}),
        _frame(5, "message.part.updated", {
            "part": {"type": "tool", "tool": "bad_tool", "state": {
                "status": "error", "title": "bad_tool", "error": "boom"}}}),
        _frame(6, "message.part.updated", {
            "part": {"type": "text", "text": "只是文本，不入轨迹"}}),
        _frame(7, "subagent.status", {"subagentId": "sa_1_1", "status": "finished"}),
        _frame(8, "session.idle", {}),
    ]


def test_extract_trace_rows_and_agent_labels():
    rows = extract_trace(_records())
    kinds = [r["kind"] for r in rows]
    assert kinds == ["subagent", "tool", "tool", "tool", "subagent"]
    tool_rows = [r for r in rows if r["kind"] == "tool"]
    assert tool_rows[0]["agent"] == "aidxf(sa_1_1)"       # persona 名册生效
    assert tool_rows[1]["agent"] == "主"                   # 无标签 = 主 agent
    assert tool_rows[0]["artifacts"] == ["skill-work/w-0001/building.json", "plan.json"]
    assert tool_rows[1]["artifacts"] == ["m_0123456789abcdef"]
    assert tool_rows[2]["status"] == "error" and tool_rows[2]["artifacts"] == []


def test_extract_trace_persona_unknown_falls_back_to_id():
    rows = extract_trace([
        _frame(1, "message.part.updated", {
            "subagentId": "sa_2_1",
            "part": {"type": "tool", "tool": "t", "state": {"status": "running", "title": "t"}}}),
    ])
    assert rows[0]["agent"] == "sa_2_1"


def test_extract_trace_task_brief_compacts_json():
    rows = extract_trace([
        _frame(1, "subagent.status", {"subagentId": "sa_1_1", "persona": "aiplan",
                                      "status": "started",
                                      "task": '{"kind": "cad", "brief": "二层小楼"}'}),
    ])
    assert "kind=cad" in rows[0]["task"] and "brief=二层小楼" in rows[0]["task"]


# ---- 渲染 --------------------------------------------------------------------

def test_trace_line_tool_with_artifacts():
    row = {"ts": "2026-09-18T12:00:01.000", "kind": "tool", "agent": "主",
           "tool": "render_model", "status": "completed",
           "artifacts": ["m_0123456789abcdef"], "task": ""}
    line = trace_line(row)
    assert "主" in line and "render_model ✓" in line and "m_0123456789abcdef" in line


def test_trace_line_subagent_and_error_states():
    sub = {"ts": "t", "kind": "subagent", "agent": "aidxf(sa_1_1)", "tool": "",
           "status": "started", "artifacts": [], "task": "生成图纸"}
    assert "⟦子agent started⟧" in trace_line(sub) and "aidxf(sa_1_1)" in trace_line(sub)
    err = {"ts": "t", "kind": "tool", "agent": "主", "tool": "bad",
           "status": "error", "artifacts": [], "task": ""}
    assert "✗" in trace_line(err) and "(无产物)" in trace_line(err)


def test_render_trace_empty_placeholder():
    assert "暂无工具调用轨迹" in render_trace([])


def test_render_trace_full_contains_header_and_rows():
    text = render_trace(_records())
    assert "产物轨迹" in text
    assert "aidxf(sa_1_1)" in text and "render_model" in text
    assert "m_0123456789abcdef" in text


def test_extract_trace_defaults_for_malformed_parts():
    rows = extract_trace([
        _frame(1, "message.part.updated", {
            "part": {"type": "tool", "tool": "bare"}}),           # 无 state
        _frame(2, "message.part.updated", {"part": {"type": "tool"}}),  # 无 tool 名
        _frame(3, "message.part.updated", "not-a-dict"),          # data 非 dict：无 part，静默跳过
    ])
    assert [r["tool"] for r in rows] == ["bare", "?"]
    assert all(r["status"] == "running" for r in rows)            # 缺状态默认 running
    assert all(r["agent"] == "主" for r in rows)                  # 无标签归主


def test_extract_artifacts_from_input_state():
    rows = extract_trace([
        _frame(1, "message.part.updated", {
            "part": {"type": "tool", "tool": "stage_script", "state": {
                "status": "running", "title": "stage_script",
                "input": '{"modelId": "m_ffffffffffffffff", "path": "scripts/v2.py"}'}}}),
    ])
    assert rows[0]["artifacts"] == ["m_ffffffffffffffff", "scripts/v2.py"]
