#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""replay.py —— 确定性离线回放（agent --replay <frames.jsonl>）。

语义：把已落盘/手工构造的帧流按序重放进帧面板与消息视图（不连 server），
用于对照回归——同帧流两跑渲染结果必须一致（帧解析/渲染/轨迹提取均为纯函数）。
scriptedModel 侧的确定性由 server 契约测试保证（W-0051）；本工具回放的是
「SSE 帧序列」这一层，与 server 是否可注入脚本无关。
"""
from pathlib import Path

from frames_store import load_frames


def load_replay(path):
    """读回放源：返回 (records, summary_text)。文件缺失/零可读帧时抛 ValueError。"""
    p = Path(path)
    if not p.is_file():
        raise ValueError(f"回放文件不存在：{p}")
    records, header, skipped = load_frames(p)
    if not records:
        raise ValueError(f"回放文件无有效帧：{p}（跳过 {skipped} 坏行）")
    src = (header or {}).get("cid") or "未知会话"
    summary = (f"回放 {len(records)} 帧（源 {src}，"
               f"{'跳过 ' + str(skipped) + ' 坏行，' if skipped else ''}{p}）")
    return records, summary


def feed_replay(records, on_frame):
    """逐帧喂给回调（同步、零延迟——确定性），返回帧数。

    on_frame(record) 每帧一次；回调异常即时上抛（回放不吞错）。
    """
    count = 0
    for rec in records:
        on_frame(rec)
        count += 1
    return count
