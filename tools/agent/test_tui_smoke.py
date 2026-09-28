#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""test_tui_smoke.py —— textual Pilot 冒烟（headless run_test）。

环境要求：tools/agent 自管 venv（agent 启动器装 textual + pytest；
textual 8.x 无独立 textual.testing 包，Pilot 经 App.run_test() 获取）。
覆盖：--replay 离线回放路径（不连 server）——帧面板逐行、消息视图渲染、
轨迹视图切换、question.ask 文本化（不弹框）。
"""
import asyncio
import json

from textual.widgets import RichLog, Static

from agent_tui import AgentApp, QuestionScreen


def _frames():
    """构造一小段契约形帧流（chat_translate.go 翻译层输出形状）。"""
    return [
        {"type": "frame", "seq": 1, "ts": "2026-09-18T12:00:00.100", "event": "message.updated",
         "sid": 1, "cid": "cs_test", "data": {"info": {"role": "user"}}},
        {"type": "frame", "seq": 2, "ts": "2026-09-18T12:00:00.200", "event": "message.part.delta",
         "sid": 2, "cid": "cs_test", "data": {"partID": "part_1_1_text", "field": "text",
                                              "delta": "开始生成"}},
        {"type": "frame", "seq": 3, "ts": "2026-09-18T12:00:01.000", "event": "subagent.status",
         "sid": 3, "cid": "cs_test", "data": {"subagentId": "sa_1_1", "parentSessionId": "cs_test",
                                              "persona": "aidxf", "status": "started",
                                              "task": "生成图纸"}},
        {"type": "frame", "seq": 4, "ts": "2026-09-18T12:00:02.000", "event": "message.part.updated",
         "sid": 4, "cid": "cs_test", "data": {"subagentId": "sa_1_1", "part": {
             "type": "tool", "tool": "execute", "state": {"status": "completed", "title": "execute",
             "output": "产物 skill-work/w-0001/building.json"}}}},
        {"type": "frame", "seq": 5, "ts": "2026-09-18T12:00:03.000", "event": "message.part.updated",
         "sid": 5, "cid": "cs_test", "data": {"part": {
             "type": "tool", "tool": "render_model", "state": {"status": "completed",
             "title": "render_model", "output": "渲染完成 m_0123456789abcdef"}}}},
        {"type": "frame", "seq": 6, "ts": "2026-09-18T12:00:04.000", "event": "question.ask",
         "sid": 6, "cid": "cs_test", "data": {"interruptId": "int_1", "question": "用哪个方案？"}},
        {"type": "frame", "seq": 7, "ts": "2026-09-18T12:00:05.000", "event": "session.idle",
         "sid": 7, "cid": "cs_test", "data": {}},
    ]


def _write_frames(tmp_path, records):
    p = tmp_path / "frames.jsonl"
    p.write_text("\n".join(json.dumps(r) for r in records) + "\n", encoding="utf-8")
    return p


def _status_text(app):
    return str(app.query_one("#status", Static).content)


async def _wait_status(app, pilot, needle):
    for _ in range(300):  # 条件等待回放 worker 完成（禁固定 sleep）
        await pilot.pause(0.01)
        if needle in _status_text(app):
            return True
    return False


def test_replay_smoke_frames_messages_trace(tmp_path):
    frames = _frames()
    path = _write_frames(tmp_path, frames)
    app = AgentApp(replay_path=str(path))

    async def body():
        async with app.run_test() as pilot:
            assert await _wait_status(app, pilot, "回放完成")
            assert f"{len(frames)} 帧" in _status_text(app)
            # ① 帧面板：全部帧入 RichLog
            log = app.query_one("#frameslog", RichLog)
            assert len(log.lines) >= len(frames)
            # ② 内存帧序列完整（轨迹源）
            assert [r["event"] for r in app.frame_records] == [f["event"] for f in frames]
            # ③ 消息视图：工具卡 + delta 流 + 子agent 行渲染
            msgs = [str(s.content) for s in app.query("#messages Static")]
            joined = "\n".join(msgs)
            assert "✓ render_model" in joined
            assert "开始生成" in joined
            assert "sa_1_1 started" in joined
            # ④ question.ask 回放文本化，不弹交互框
            assert "（回放）用哪个方案？" in joined
            assert not isinstance(app.screen, QuestionScreen)
            # ⑤ 轨迹视图：切换后提取出 agent→tool→产物
            app.action_trace()
            await pilot.pause()
            trace = str(app.query_one("#tracebody", Static).content)
            assert "产物轨迹" in trace
            assert "aidxf(sa_1_1)" in trace and "execute ✓" in trace
            assert "m_0123456789abcdef" in trace and "building.json" in trace
            # ⑥ 回放不落盘（无二次写出）
            assert app.recorder is None and app.frames_path is None
            # ⑦ 回放模式输入禁用（无 cid 可发）
            from textual.widgets import Input
            assert app.query_one("#chatinput", Input).disabled

    asyncio.run(body())


def test_replay_bad_file_shows_error(tmp_path):
    app = AgentApp(replay_path=str(tmp_path / "missing.jsonl"))

    async def body():
        async with app.run_test() as pilot:
            assert await _wait_status(app, pilot, "回放失败")
            assert app.frame_records == []

    asyncio.run(body())
