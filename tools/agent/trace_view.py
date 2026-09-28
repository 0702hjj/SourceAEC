#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""trace_view.py —— 产物落盘轨迹：帧流 → 「哪个 agent → 调了什么 tool → 产物路径」时间线。

数据源：frames_store 的帧记录（实时累积或回放加载）。提取规则：
  - subagent.status 帧：persona 名册（sa_{turn}_{seq} → persona），并生成子边界行；
  - message.part.updated 且 part.type=="tool"：工具行（agent 归属看 data.subagentId，
    无标签 = 主 agent；input/output/error 里提取产物）。
产物识别（input/output 尽力而为，正则扫描字符串）：
  - modelId：m_[0-9a-f]{16}
  - 文件路径：带扩展名（.json/.ifc/.dxf/.py/.md/.png/.txt）的路径串，含 plans/、skill-work/
"""
import json
import re

MODEL_ID_RE = re.compile(r"\bm_[0-9a-f]{16}\b")
PATH_RE = re.compile(r"[A-Za-z0-9_][A-Za-z0-9_./~-]*\.(?:json|ifc|dxf|py|md|png|txt)")
ARTIFACT_ORDER = ("json", "ifc", "dxf", "py", "md", "png", "txt")

STATUS_ICON = {"running": "◌", "completed": "✓", "error": "✗"}
STATUS_COLOR = {"running": "yellow", "completed": "green", "error": "red"}


def extract_artifacts(text):
    """从 input/output 字符串提取产物（modelId + 文件路径），按出现序去重。"""
    if not text:
        return []
    found, seen = [], set()
    for m in MODEL_ID_RE.finditer(text):
        if m.group(0) not in seen:
            seen.add(m.group(0))
            found.append(m.group(0))
    for m in PATH_RE.finditer(text):
        p = m.group(0).rstrip(".,;:)\"']")
        if p not in seen:
            seen.add(p)
            found.append(p)
    # 排序稳定化：modelId 最前，其余按 (扩展序, 首现位置)
    def order(a):
        ext = a.rsplit(".", 1)[-1] if "." in a else ""
        return (0 if MODEL_ID_RE.fullmatch(a) else 1,
                ARTIFACT_ORDER.index(ext) if ext in ARTIFACT_ORDER else 99)
    return sorted(found, key=order)


def _part_of(data):
    part = (data or {}).get("part") if isinstance(data, dict) else None
    return part if isinstance(part, dict) else {}


def _state_of(part):
    st = part.get("state")
    return st if isinstance(st, dict) else {}


def _clock(ts):
    ts = str(ts or "")
    return ts[11:19] if len(ts) >= 19 else ts


def extract_trace(records):
    """帧记录序列 → 轨迹行列表。

    行形（dict）：{ts, kind: "subagent"|"tool", agent, tool, status, artifacts, task}
    agent 标签：主 agent = "主"；子 = "{persona}({subagentId})"（persona 未知则仅 id）。
    """
    personas = {}  # subagentId -> persona（subagent.status 帧先到先记）
    rows = []
    for rec in records:
        data = rec.get("data") or {}
        event = rec.get("event") or ""
        if event == "subagent.status":
            sid = data.get("subagentId", "")
            if data.get("persona"):
                personas[sid] = data.get("persona")
            rows.append({
                "ts": rec.get("ts", ""), "kind": "subagent",
                "agent": _agent_label(personas, sid), "tool": "",
                "status": data.get("status", ""), "artifacts": [],
                "task": _task_brief(data.get("task", "")),
            })
            continue
        if event != "message.part.updated":
            continue
        part = _part_of(data)
        if part.get("type") != "tool":
            continue
        sid = data.get("subagentId", "")
        st = _state_of(part)
        blob = " ".join(str(x or "") for x in (st.get("input"), st.get("output"), st.get("error")))
        rows.append({
            "ts": rec.get("ts", ""), "kind": "tool",
            "agent": _agent_label(personas, sid),
            "tool": st.get("title") or part.get("tool") or "?",
            "status": st.get("status", "running"),
            "artifacts": extract_artifacts(blob),
            "task": "",
        })
    return rows


def _agent_label(personas, sid):
    if not sid:
        return "主"
    persona = personas.get(sid, "")
    return f"{persona}({sid})" if persona else sid


def _task_brief(task, maxlen=64):
    text = " ".join(str(task or "").split())
    try:  # task 常是 JSON 参数串，取几个关键字段压缩显示
        obj = json.loads(text)
        if isinstance(obj, dict):
            text = " ".join(f"{k}={v}" for k, v in list(obj.items())[:3])
    except ValueError:
        pass
    return text if len(text) <= maxlen else text[:maxlen - 3] + "..."


def trace_line(row):
    """轨迹行 → 单行 markup（时间线视图用，回放/实时共用）。"""
    icon = STATUS_ICON.get(row["status"], "·")
    color = STATUS_COLOR.get(row["status"], "white")
    head = f"[dim]{_clock(row['ts'])}[/] [bold]{row['agent']}[/]"
    if row["kind"] == "subagent":
        task = f" [dim]{row['task']}[/]" if row["task"] else ""
        return f"{head} [magenta]⟦子agent {row['status']}⟧[/]{task}"
    line = f"{head} → [{color}]{row['tool']} {icon}[/]"
    if row["artifacts"]:
        arts = " ".join(f"[underline cyan]{a}[/]" for a in row["artifacts"])
        line += f" → {arts}"
    elif row["status"] == "error":
        line += " [red](无产物)[/]"
    return line


def render_trace(records):
    """帧记录 → 轨迹视图整段 markup（无产物行提示）。"""
    rows = extract_trace(records)
    if not rows:
        return "[dim]（暂无工具调用轨迹——对话产生 tool 帧后此处生成时间线）[/]"
    lines = ["[bold]产物轨迹（agent → tool → 产物）[/]"]
    lines += [trace_line(r) for r in rows]
    return "\n".join(lines)
