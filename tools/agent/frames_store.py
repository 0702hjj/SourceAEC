#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""frames_store.py —— SSE 原始帧的记录、JSONL 落盘与单行渲染（纯函数，无 textual）。

帧契约（server/internal/api/chat_sse.go + chat_translate.go）：
  id: <n>            会话内递增序号（Last-Event-ID 重同步用）
  event: <kind>      session.status | message.updated | message.part.updated |
                     message.part.delta | subagent.status | question.ask |
                     session.error | session.idle
  data: <json>       子 agent 帧带 subagentId 字段（sa_{turn}_{seq}）

落盘格式（JSONL，一行一帧；首行 header）：
  {"type": "header", "cid": ..., "created_at": ..., "source": "live"|"replay-input"}
  {"type": "frame", "seq": 1, "ts": "...", "event": "...", "sid": 3, "cid": ..., "data": {...}}

落盘目录默认 ~/.cache/aiifc-agent/frames/（env AIIFC_AGENT_FRAMES_DIR 可覆盖；
不在 git 跟踪目录、不进 data/——AGENTS 硬规则）。
"""
import json
import os
import re
from datetime import datetime
from pathlib import Path

from runtime import AGENT_CACHE_DIR

PREVIEW_MAX = 96  # 帧行 data 预览截断长度（字符）
TRUNC_MARK = "...(truncated)"

# 按 kind 着色（textual markup 颜色名）
KIND_COLORS = {
    "session.status": "dim",
    "session.idle": "dim",
    "message.updated": "blue",
    "message.part.updated": "cyan",
    "message.part.delta": "grey54",
    "subagent.status": "magenta",
    "question.ask": "bold yellow",
    "session.error": "bold red",
}
DEFAULT_COLOR = "white"


def frames_dir() -> Path:
    d = Path(os.environ.get("AIIFC_AGENT_FRAMES_DIR", AGENT_CACHE_DIR / "frames"))
    d.mkdir(parents=True, exist_ok=True)
    return d


def new_frames_path(cid="session") -> Path:
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    return frames_dir() / f"{cid}-{stamp}.jsonl"


class FrameRecorder:
    """逐帧 append JSONL（每行 flush，turn 中断也不丢已收帧）。"""

    def __init__(self, path, cid="", source="live"):
        self.path = Path(path)
        self.cid = cid
        self.seq = 0
        self._f = open(self.path, "a", encoding="utf-8")
        if self._f.tell() == 0:
            self._write({"type": "header", "cid": cid, "created_at": _now(),
                         "source": source})

    def record(self, event, data, sid=None, ts=None):
        self.seq += 1
        rec = {"type": "frame", "seq": self.seq, "ts": ts or _now(),
               "event": event, "sid": sid, "cid": self.cid, "data": data}
        self._write(rec)
        return rec

    def _write(self, obj):
        self._f.write(json.dumps(obj, ensure_ascii=False) + "\n")
        self._f.flush()

    def close(self):
        if not self._f.closed:
            self._f.close()


def _now():
    return datetime.now().isoformat(timespec="milliseconds")


def load_frames(path):
    """读回帧序列（回放用）：跳过 header/坏行，返回 (records, header, skipped)。

    兼容两种行形：本工具落盘的 {"type":"frame",...} 与裸 SSE 三元组
    {"event":..., "sid":..., "data":...}（手工构造的最小帧流）。
    """
    records, header, skipped = [], None, 0
    seq = 0
    for line in Path(path).read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            rec = json.loads(line)
        except ValueError:
            skipped += 1
            continue
        if not isinstance(rec, dict):
            skipped += 1
            continue
        if rec.get("type") == "header":
            header = rec
            continue
        if rec.get("type") not in (None, "frame") or "event" not in rec:
            skipped += 1
            continue
        seq += 1
        records.append({
            "seq": rec.get("seq", seq), "ts": rec.get("ts", ""),
            "event": rec.get("event"), "sid": rec.get("sid"),
            "data": rec.get("data") or {},
        })
    return records, header, skipped


def truncate_preview(data, maxlen=PREVIEW_MAX):
    """帧 data 的紧凑预览；超长截断并带 ...(truncated) 标记。返回 (preview, truncated)。"""
    try:
        text = json.dumps(data, ensure_ascii=False, separators=(",", ":"))
    except (TypeError, ValueError):
        text = str(data)
    text = " ".join(text.split())  # 压掉换行（单行显示）
    if len(text) > maxlen:
        return text[:maxlen] + TRUNC_MARK, True
    return text, False


_SUBID_RE = re.compile(r"\bsa_\d+_\d+\b")


def sub_label(data):
    """子事件标签：data.subagentId（sa_{turn}_{seq}），无则空串。"""
    sid = (data or {}).get("subagentId") if isinstance(data, dict) else None
    if isinstance(sid, str) and _SUBID_RE.match(sid):
        return sid
    return ""


def kind_color(event):
    return KIND_COLORS.get(event or "", DEFAULT_COLOR)


def frame_line(rec):
    """一帧 → 单行 markup（序号 | 时间 | kind | 子标签 | 预览）。纯函数，回放/实时共用。"""
    seq = rec.get("seq", 0)
    ts = str(rec.get("ts", ""))
    clock = ts[11:23] if len(ts) >= 23 else ts  # 只取 HH:MM:SS.mmm
    event = rec.get("event") or "?"
    color = kind_color(event)
    parts = [f"[dim]#{seq:04d}[/] [dim]{clock}[/] [{color}]{event}[/]"]
    label = sub_label(rec.get("data"))
    if label:
        parts.append(f"[magenta]{label}[/]")
    preview, truncated = truncate_preview(rec.get("data"))
    if preview:
        if truncated:
            parts.append(f"[dim]{preview}[/]")
        else:
            parts.append(preview)
    return " ".join(parts)
