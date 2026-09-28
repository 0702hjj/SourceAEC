#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""frames_pane.py —— TUI 侧栏：SSE 原始帧面板 / 产物轨迹视图 二选一切换。

键位（AgentApp BINDINGS 委托到本组件）：
  f —— 帧面板（每帧一行：序号/时间/kind/子标签/预览，按 kind 着色）
  t —— 轨迹视图（agent → tool → 产物 时间线，trace_view 渲染）
  h —— 侧栏显隐
"""
from textual.containers import Vertical
from textual.widgets import RichLog, Static

import trace_view

MAX_FRAME_LINES = 2000  # RichLog 环形上限（防长会话内存增长不受控）


class SideBar(Vertical):
    """右侧调试侧栏：#sidehead 标题行 + 两个互斥视图（frames RichLog / trace Static）。"""

    DEFAULT_CSS = """
    SideBar { width: 46; min-width: 30; border: round $accent; display: block; }
    #sidehead { height: 2; padding: 0 1; background: $surface; }
    #frameslog { height: 1fr; }
    #tracebody { height: 1fr; overflow-y: auto; padding: 0 1; }
    """

    def __init__(self, **kw):
        super().__init__(**kw)
        self.view = "frames"
        self.frame_count = 0

    def compose(self):
        yield Static("", id="sidehead", markup=True)
        yield RichLog(markup=True, max_lines=MAX_FRAME_LINES, id="frameslog",
                      wrap=True, highlight=False)
        yield Static("", id="tracebody", markup=True)

    def on_mount(self) -> None:
        self.show_frames()

    # -- 帧面板 ----------------------------------------------------------------

    def append_frame_line(self, line: str) -> None:
        self.query_one("#frameslog", RichLog).write(line)
        self.frame_count += 1
        self._refresh_head()

    def clear_frames(self) -> None:
        self.query_one("#frameslog", RichLog).clear()
        self.frame_count = 0
        self._refresh_head()

    # -- 轨迹视图 --------------------------------------------------------------

    def render_trace(self, records) -> None:
        self.query_one("#tracebody", Static).update(trace_view.render_trace(records))

    # -- 视图切换 --------------------------------------------------------------

    def show_frames(self) -> None:
        self.view = "frames"
        self.query_one("#tracebody", Static).display = False
        log = self.query_one("#frameslog", RichLog)
        log.display = True
        log.scroll_end(animate=False)
        self._refresh_head()

    def show_trace(self, records=None) -> None:
        self.view = "trace"
        if records is not None:
            self.render_trace(records)
        self.query_one("#frameslog", RichLog).display = False
        self.query_one("#tracebody", Static).display = True
        self._refresh_head()

    def toggle_visible(self) -> None:
        self.display = not self.display

    @property
    def visible(self) -> bool:
        return self.display

    def set_badge(self, text: str) -> None:
        """标题前缀徽标（LLM 模式 / 回放来源），存起来 _refresh_head 拼装。"""
        self.badge = text
        self._refresh_head()

    badge = ""

    def _refresh_head(self) -> None:
        which = "帧流" if self.view == "frames" else "轨迹"
        badge = f" {self.badge}" if self.badge else ""
        hint = " [f]帧 [t]轨迹 [h]隐" if self.display else " [h]显"
        self.query_one("#sidehead", Static).update(
            f"[bold]{which}[/] · {self.frame_count} 帧{badge}{hint}")
