#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""test_replay.py —— 回放加载与逐帧喂给 单测。"""
import json

import pytest
from replay import feed_replay, load_replay


def _write(path, records, cid="cs_test"):
    lines = [json.dumps({"type": "header", "cid": cid, "created_at": "t", "source": "live"})]
    lines += [json.dumps(r) for r in records]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return path


def test_load_replay_missing_file(tmp_path):
    with pytest.raises(ValueError, match="不存在"):
        load_replay(tmp_path / "nope.jsonl")


def test_load_replay_no_valid_frames(tmp_path):
    p = tmp_path / "empty.jsonl"
    p.write_text("garbage\n" + json.dumps({"type": "header", "cid": "x"}) + "\n")
    with pytest.raises(ValueError, match="无有效帧"):
        load_replay(p)


def test_load_replay_summary_and_records(tmp_path):
    p = _write(tmp_path / "f.jsonl", [
        {"type": "frame", "seq": 1, "ts": "2026-09-18T00:00:00.000",
         "event": "session.idle", "sid": 1, "cid": "cs_test", "data": {}},
    ])
    records, summary = load_replay(p)
    assert len(records) == 1 and records[0]["event"] == "session.idle"
    assert "回放 1 帧" in summary and "cs_test" in summary and "0 坏行" not in summary


def test_load_replay_reports_skipped_lines(tmp_path):
    p = tmp_path / "mixed.jsonl"
    p.write_text("\n".join([
        json.dumps({"type": "header", "cid": "c"}),
        "broken line",
        json.dumps({"event": "session.idle", "data": {}}),
    ]) + "\n")
    records, summary = load_replay(p)
    assert len(records) == 1 and "跳过 1 坏行" in summary


def test_feed_replay_orders_and_counts():
    seen = []
    records = [{"event": f"e{i}", "data": {}} for i in range(5)]
    assert feed_replay(records, seen.append) == 5
    assert [r["event"] for r in seen] == [f"e{i}" for i in range(5)]


def test_feed_replay_propagates_callback_error():
    def boom(rec):
        raise RuntimeError("callback fail")
    with pytest.raises(RuntimeError, match="callback fail"):
        feed_replay([{"event": "x", "data": {}}], boom)
