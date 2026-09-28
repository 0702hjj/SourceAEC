#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""setup_screen.py —— agent TUI 的 SetupScreen（项目选择/新建/删除）。

从 agent_tui.py 拆出（W-0049 行数门控）：历史项目（会话）列表选择、
n 新建项目、d 序号删除项目。依赖 agent_tui 的 http_json / SERVER_PORT。
"""
import time
import urllib.error

from textual import work
from textual.app import ComposeResult
from textual.screen import ModalScreen
from textual.widgets import Input, Static


class SetupScreen(ModalScreen):
    """项目选择：历史项目（会话）列表优先，选中即进入项目会话；或 n 新建项目。

    接口与前端 LibraryPage 一致（GET /api/v1/chat/sessions 拉历史会话）。
    dismiss 结果：
      ("session", chatSessionId, projectId) —— 进历史项目会话
      ("new", title, kind)                  —— 新建项目 + 会话
    """

    def __init__(self, **kw):
        super().__init__(**kw)
        self.sessions = []
        self.mode = "pick"  # pick -> new

    def compose(self) -> ComposeResult:
        yield Static("加载历史项目…", id="list")
        yield Static("")
        yield Static("输入序号进入 / d 序号删除 / n 新建", id="hint")
        yield Input(placeholder="n", id="pick")
        yield Static("", id="formtitle")
        yield Input(placeholder="项目名（回车=未命名项目）", id="title")
        yield Input(placeholder="项目类型：1=cad 2=ifc 3=cad->ifc（默认 1）", id="kind")

    def on_mount(self) -> None:
        self.query_one("#title", Input).display = False
        self.query_one("#kind", Input).display = False
        self.query_one("#title", Input).styles.display = "none"
        self.query_one("#kind", Input).styles.display = "none"
        self.query_one("#formtitle", Static).update("")
        self.load_sessions()

    @work(thread=True)
    def load_sessions(self) -> None:
        from agent_tui import SERVER_PORT, http_json  # 延迟 import（防循环）
        # 等服务就绪（后台 prepare_services 拉起中），再拉历史项目会话列表
        app = self.app
        deadline = time.time() + 95
        while not getattr(app, "services_ready", False):
            if time.time() > deadline:
                break
            time.sleep(0.5)
        try:
            r = http_json("GET", SERVER_PORT, "/api/v1/chat/sessions", timeout=10)
            self.sessions = r.get("data") or []
        except Exception as e:
            self.sessions = []
            app.call_from_thread(self.query_one("#list", Static).update,
                                 f"[red]拉历史项目失败：{e}[/]\n[dim]可直接 n 新建[/]")
            app.call_from_thread(self.query_one("#pick", Input).focus)
            return
        if not self.sessions:
            app.call_from_thread(self.query_one("#list", Static).update,
                                 "[dim]（暂无历史项目，输 n 新建）[/]")
        else:
            lines = ["[bold]历史项目（会话）——点序号进入：[/]"]
            for i, s in enumerate(self.sessions, 1):
                pid = s.get("projectId") or "-"
                when = s.get("createdAt", "")[:16].replace("T", " ")
                lines.append(f"  [{i}] {s.get('title','未命名')}  [dim]({pid} · {when})[/]")
            app.call_from_thread(self.query_one("#list", Static).update, "\n".join(lines))
        app.call_from_thread(self.query_one("#pick", Input).focus)

    def on_input_submitted(self, event: Input.Submitted) -> None:
        val = event.value.strip()
        if self.mode == "pick":
            if val.lower() == "n":
                self.to_new_form()
                return
            # 删除命令在无项目列表时明确提示（不静默走新建——UX bug）。
            if val.lower().startswith("d") and not self.sessions:
                self.query_one("#hint", Static).update("[yellow]暂无项目可删[/]（历史项目列表为空，输 n 新建）")
                self.query_one("#pick", Input).value = ""
                return
            if not self.sessions:
                self.to_new_form()
                return
            # d <序号> 删除项目（级联：项目+会话+方案+项目下模型）
            if val.lower().startswith("d"):
                try:
                    idx = int(val[1:].strip()) - 1
                    s = self.sessions[idx]
                except (ValueError, IndexError):
                    self.query_one("#hint", Static).update("[red]无效删除序号[/] 如 d 1")
                    self.query_one("#pick", Input).value = ""
                    return
                self.delete_project(s)
                return
            try:
                idx = int(val) - 1
                s = self.sessions[idx]
                self.dismiss(("session", s.get("chatSessionId"), s.get("projectId")))
            except (ValueError, IndexError):
                self.query_one("#hint", Static).update("[red]无效序号[/] 输入序号进入 / d 序号删除 / n 新建")
                self.query_one("#pick", Input).value = ""
            return
        if event.input.id == "title":
            self.query_one("#kind", Input).focus()
            return
        title = self.query_one("#title", Input).value.strip() or "未命名项目"
        kind = {"1": "cad", "2": "ifc", "3": "cad->ifc"}.get(
            self.query_one("#kind", Input).value.strip() or "1")
        if kind is None:
            self.query_one("#kind", Input).value = ""
            return
        self.dismiss(("new", title, kind))

    @work(thread=True)
    def delete_project(self, session: dict) -> None:
        from agent_tui import SERVER_PORT, http_json  # 延迟 import（防循环）
        """删除项目（级联），删除后刷新历史项目列表。"""
        pid = session.get("projectId")
        title = session.get("title", pid)
        # pid 缺失/空 → 无法删除（孤儿会话/字段缺失），提示并刷新列表兜底。
        if not pid:
            self.app.call_from_thread(self.query_one("#hint", Static).update,
                                      f"[red]项目 id 缺失，无法删除（可能已是孤儿会话）[/]")
            self.load_sessions()
            return
        try:
            http_json("DELETE", SERVER_PORT, f"/api/v1/chat/projects/{pid}", timeout=15)
        except urllib.error.HTTPError as e:
            # 404 = 项目已不存在（孤儿会话）——从列表兜底移除（前端 web 同语义）。
            if e.code == 404:
                self.app.call_from_thread(self.query_one("#hint", Static).update,
                                          f"[yellow]项目「{title}」已不存在（404），从列表移除[/]")
            else:
                self.app.call_from_thread(self.query_one("#hint", Static).update,
                                          f"[red]删除失败：{e}[/]")
            self.load_sessions()
            return
        except Exception as e:
            self.app.call_from_thread(self.query_one("#hint", Static).update,
                                      f"[red]删除失败：{e}[/]")
            return
        self.app.call_from_thread(self.query_one("#hint", Static).update,
                                  f"[green]已删除项目「{title}」[/]（级联：会话/方案/模型）")
        self.load_sessions()

    def to_new_form(self) -> None:
        self.mode = "new"
        self.query_one("#hint", Static).update("新建项目（kind 决定 Agent 派发方向）")
        self.query_one("#pick", Input).styles.display = "none"
        self.query_one("#formtitle", Static).update("项目名 + 项目类型：")
        self.query_one("#title", Input).styles.display = "block"
        self.query_one("#kind", Input).styles.display = "block"
        self.query_one("#title", Input).focus()
