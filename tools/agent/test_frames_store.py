#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""test_frames_store.py —— SSE 解析 / 帧落盘往返 / 帧行渲染 纯函数单测。"""
import json

import frames_store
from frames_store import (TRUNC_MARK, FrameRecorder, frame_line, frames_dir,
                          kind_color, load_frames, sub_label, truncate_preview)
from runtime import parse_sse_lines


# ---- SSE 行解析（与 chat_sse.go 帧格式对齐） ---------------------------------

def test_parse_sse_basic_frame():
    frames = list(parse_sse_lines([
        "id: 3\n", "event: message.part.updated\n",
        'data: {"part": {"type": "text"}}\n', "\n",
    ]))
    assert frames == [(("message.part.updated"), 3, {"part": {"type": "text"}})]


def test_parse_sse_multiline_data_and_no_id():
    frames = list(parse_sse_lines([
        "event: message.part.delta\n",
        'data: {"delta": "a",\n', 'data: "x": 1}\n', "\n",
    ]))
    assert frames == [("message.part.delta", None, {"delta": "a", "x": 1})]


def test_parse_sse_non_json_data_wrapped_raw():
    frames = list(parse_sse_lines(["event: session.error\n", "data: not-json\n", "\n"]))
    assert frames == [("session.error", None, {"raw": "not-json"})]


def test_parse_sse_ignores_comment_and_bad_id():
    frames = list(parse_sse_lines([
        ": connected\n", "id: abc\n", "event: session.idle\n", "data: {}\n", "\n",
    ]))
    assert frames == [("session.idle", None, {})]


def test_parse_sse_trailing_frame_without_blank_line():
    frames = list(parse_sse_lines(["event: session.idle\n", "data: {}\n"]))
    assert frames == [("session.idle", None, {})]


# ---- 落盘 / 读回 --------------------------------------------------------------

def test_recorder_jsonl_roundtrip(tmp_path):
    path = tmp_path / "f.jsonl"
    rec = FrameRecorder(path, cid="cs_abc")
    rec.record("message.updated", {"info": {"role": "user"}}, sid=1)
    rec.record("session.idle", {}, sid=2)
    rec.close()
    lines = path.read_text().splitlines()
    assert len(lines) == 3
    header = json.loads(lines[0])
    assert header["type"] == "header" and header["cid"] == "cs_abc"
    records, loaded_header, skipped = load_frames(path)
    assert skipped == 0 and loaded_header["cid"] == "cs_abc"
    assert [r["event"] for r in records] == ["message.updated", "session.idle"]
    assert records[0]["seq"] == 1 and records[1]["seq"] == 2
    assert records[0]["sid"] == 1 and records[0]["data"]["info"]["role"] == "user"


def test_load_frames_bare_shape_and_bad_lines(tmp_path):
    path = tmp_path / "bare.jsonl"
    path.write_text("\n".join([
        "not json at all",
        '{"event": "session.idle", "data": {}}',
        '{"type": "note", "event": "x"}',   # 未知 type → 坏行
        '{"event": "message.updated", "data": {"info": {}}}',
    ]) + "\n")
    records, header, skipped = load_frames(path)
    assert header is None and skipped == 2
    assert [r["event"] for r in records] == ["session.idle", "message.updated"]
    assert [r["seq"] for r in records] == [1, 2]  # 缺 seq 时按序补


# ---- 预览截断 / 帧行渲染 -------------------------------------------------------

def test_truncate_preview_short_intact():
    text, truncated = truncate_preview({"a": 1})
    assert not truncated and text == '{"a":1}'


def test_truncate_preview_long_marks_truncated():
    data = {"blob": "x" * 300}
    text, truncated = truncate_preview(data)
    assert truncated and text.endswith(TRUNC_MARK)
    assert len(text) <= frames_store.PREVIEW_MAX + len(TRUNC_MARK)


def test_frame_line_contains_seq_kind_sublabel_and_truncation():
    rec = {"seq": 12, "ts": "2026-09-18T12:34:56.789", "event": "message.part.delta",
           "sid": 5, "data": {"subagentId": "sa_1_2", "delta": "y" * 200}}
    line = frame_line(rec)
    assert "#0012" in line
    assert "12:34:56.789" in line
    assert "message.part.delta" in line
    assert "sa_1_2" in line
    assert TRUNC_MARK in line


def test_frame_line_kind_coloring():
    rec = {"seq": 1, "ts": "2026-09-18T00:00:00.000", "event": "session.error",
           "sid": None, "data": {"error": "boom"}}
    line = frame_line(rec)
    assert "[bold red]session.error[/]" in line
    rec2 = {**rec, "event": "unknown.kind"}
    assert "[white]unknown.kind[/]" in frame_line(rec2)


def test_sub_label_rules():
    assert sub_label({"subagentId": "sa_3_11"}) == "sa_3_11"
    assert sub_label({"subagentId": "sa_xx"}) == ""
    assert sub_label({}) == ""
    assert sub_label(None) == ""


def test_kind_color_map_covers_contract_kinds():
    for k in ("session.status", "session.idle", "message.updated",
              "message.part.updated", "message.part.delta", "subagent.status",
              "question.ask", "session.error"):
        assert kind_color(k) != "white"
    assert kind_color(None) == "white"


def test_frames_dir_env_override(tmp_path, monkeypatch):
    monkeypatch.setenv("AIIFC_AGENT_FRAMES_DIR", str(tmp_path / "fx"))
    d = frames_dir()
    assert d == tmp_path / "fx" and d.is_dir()


def test_recorder_append_keeps_single_header(tmp_path):
    path = tmp_path / "f.jsonl"
    FrameRecorder(path, cid="c").record("session.idle", {})
    FrameRecorder(path, cid="c").record("session.idle", {})  # 二次打开（同 run 追加）
    lines = path.read_text().splitlines()
    headers = [l for l in lines if '"type": "header"' in l or '"type":"header"' in l]
    assert len(headers) == 1 and len(lines) == 3


def test_frame_line_empty_ts_and_data():
    line = frame_line({"seq": 1, "ts": "", "event": "session.idle", "sid": None, "data": {}})
    assert "#0001" in line and "session.idle" in line


def test_parse_sse_crlf_endings():
    frames = list(parse_sse_lines(["event: session.idle\r\n", "data: {}\r\n", "\r\n"]))
    assert frames == [("session.idle", None, {})]
