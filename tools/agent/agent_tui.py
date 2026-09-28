#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""agent_tui.py —— opencode 风格的终端交互（textual TUI）。

区分显示：思考内容（reasoning，灰色斜体）/ 实际回复（text，正常色）/
工具调用（独立 box 框：running→completed/error，含输入输出）。
流程与 server 实际设计一致：创建项目（kind 必选）→ 项目会话 → 对话（SSE）→ HITL。

W-0055 增强：
  --scripted           拉起 server 时用清空 llmAPIKey 的派生配置（scriptedModel 离线）
  --replay <jsonl>     离线回放已落盘帧流（不连 server，对照回归）
  侧栏（f5 帧面板 / f6 产物轨迹 / f7 显隐）：SSE 原始帧逐行可视化 + JSONL 落盘。

运行：python3 agent_tui.py [--scripted | --replay frames.jsonl]（需 textual）
依赖服务：edit :8100 / cad :8200 / server :8090（未起自动拉起，端口一一对应）。
"""
import argparse
import json
import sys
import time

from textual import work
from setup_screen import SetupScreen
from frames_pane import SideBar
from textual.app import App, ComposeResult
from textual.containers import Horizontal, VerticalScroll
from textual.screen import ModalScreen
from textual.widgets import Button, Footer, Header, Input, Static

import frames_store
import replay as replay_mod
from runtime import (SERVER_PORT, http_json, ensure_services, sse_frames,
                     detect_llm_mode)

# re-export：setup_screen 延迟 import agent_tui 的这两个名字（防循环）
__all__ = ["AgentApp", "SERVER_PORT", "http_json"]


class QuestionScreen(ModalScreen):
    """HITL：ask_user 提问，收集回答。"""

    def __init__(self, question: str, **kw):
        super().__init__(**kw)
        self.question = question

    def compose(self) -> ComposeResult:
        yield Static(f"[bold yellow]❓ 需要确认[/]\n{self.question}", id="qtext")
        yield Input(placeholder="输入回答后回车", id="answer")

    def on_input_submitted(self, event: Input.Submitted) -> None:
        self.dismiss(event.value.strip())


class AgentApp(App):
    TITLE = "AI_IFC Agent"
    SUB_TITLE = "opencode 风格终端"
    BINDINGS = [
        ("f2", "abort", "中止 turn"),
        ("f5", "frames", "帧面板"),
        ("f6", "trace", "产物轨迹"),
        ("f7", "sidebar", "侧栏显隐"),
        ("q", "quit", "退出（自动中止）"),
    ]
    CSS = """
    Screen { layout: vertical; }
    #status { height: 1; background: $surface; color: $text; padding: 0 1; }
    #main { height: 1fr; }
    #messages { width: 1fr; border: round $primary; overflow-y: auto; }
    #messages Static { padding: 0 1; }
    #inputrow { height: 3; }
    #inputrow Input { width: 1fr; margin: 0 0 0 1; }
    #abortbtn { width: 10; margin: 0 1 0 0; }
    """

    def on_button_pressed(self, event: Button.Pressed) -> None:
        """UI 按钮：中止按钮点击 → 中止当前 turn。"""
        if event.button.id == "abortbtn":
            self.abort_turn()

    def action_abort(self) -> None:
        """全局按键：中止当前会话 turn（UI 层显式暴露 abort 接口）。"""
        self.abort_turn()

    def action_frames(self) -> None:
        self.query_one("#sidebar", SideBar).show_frames()

    def action_trace(self) -> None:
        self.query_one("#sidebar", SideBar).show_trace(self.frame_records)

    def action_sidebar(self) -> None:
        self.query_one("#sidebar", SideBar).toggle_visible()

    def action_quit(self) -> None:
        """全局 ctrl+q：退出（先中止当前 turn，避免模型后台残留）。"""
        self.abort_turn()
        self.exit()

    def __init__(self, scripted: bool = False, replay_path: str = None):
        super().__init__()
        self.cid = self.pid = None
        self.streams = {}  # partID -> (Static, accumulated_text)
        self.services_ready = False
        self.service_error = ""
        self.busy = False  # 模型 turn 运行中（abort 按钮可用态）
        self.scripted = scripted
        self.replay_path = replay_path
        self.frame_records = []  # 内存帧序列（轨迹提取源）
        self.recorder = None     # JSONL 落盘（首帧惰性创建）
        self.frames_path = None

    def compose(self) -> ComposeResult:
        yield Header()
        yield Static("…", id="status")
        with Horizontal(id="main"):
            yield VerticalScroll(id="messages")
            yield SideBar(id="sidebar")
        yield Footer()
        with Horizontal(id="inputrow"):
            yield Input(placeholder="输入消息（/plans 看方案历史 /quit 退出）", id="chatinput")
            yield Button("中止", id="abortbtn", variant="error")

    def on_mount(self) -> None:
        self.query_one("#chatinput", Input).disabled = True
        self.query_one("#abortbtn", Button).disabled = True
        if self.replay_path:
            self.set_status("回放模式（离线，不连 server）")
            self.run_replay(self.replay_path)
            return
        # 立即弹首次设置表单（不等服务），服务后台准备——用户马上能打字
        mode, detail = detect_llm_mode()
        if self.scripted:
            mode = "scripted"
        self.query_one("#sidebar", SideBar).set_badge(mode)
        self.set_status(f"模式 {mode}（{detail}）· 服务启动中…")
        self.prepare_services()
        self.push_screen(SetupScreen(), self.on_setup_result)

    @work(thread=True)
    def prepare_services(self) -> None:
        """后台拉起依赖服务（edit/cad/server），start_project 前轮询就绪。"""
        try:
            warning = ensure_services(scripted=self.scripted)
            self.services_ready = True
            self.service_error = ""
            if warning:
                self.call_from_thread(self.emit, f"[yellow]⚠ {warning}[/]")
        except RuntimeError as e:
            self.services_ready = False
            self.service_error = str(e)

    def on_setup_result(self, result) -> None:
        """SetupScreen 提交回调：("session", cid, pid) → 历史会话；("new", title, kind) → 新建。"""
        if not result:
            return
        if result[0] == "session":
            _, cid, pid = result
            self.cid, self.pid = cid, pid
            self.enter_session(cid, pid)
        else:
            _, title, kind = result
            self.start_project(title, kind)

    @work(thread=True)
    def enter_session(self, cid: str, pid: str) -> None:
        """进入历史项目会话：等服务就绪（后台 prepare_services）+ 回填历史对话。"""
        deadline = time.time() + 95
        while not self.services_ready:
            if time.time() > deadline:
                self.call_from_thread(self.emit, f"[red]服务未就绪：{self.service_error}[/]")
                return
            time.sleep(0.5)
        self.call_from_thread(self.emit,
            f"[bold green]✓ 进入历史项目会话 {cid}（项目 {pid}）[/]")
        self.call_from_thread(self.ui_ready, f"项目 {pid} · 会话 {cid}")
        self.load_history(cid)

    @work(thread=True)
    def load_history(self, cid: str) -> None:
        """回填历史对话（GET /chat/sessions/{cid}/messages → {info, parts} 渲染）。"""
        try:
            r = http_json("GET", SERVER_PORT, f"/api/v1/chat/sessions/{cid}/messages", timeout=10)
        except Exception as e:
            self.call_from_thread(self.emit, f"[dim]历史加载失败：{e}[/]")
            return
        msgs = r.get("data") or []
        if not msgs:
            self.call_from_thread(self.emit, "[dim]（新会话，无历史）[/]")
            return
        lines = ["[dim]—— 历史对话 ——[/]"]
        for m in msgs:
            role = (m.get("info") or {}).get("role", "")
            for p in m.get("parts") or []:
                ptype = p.get("type")
                if ptype == "text" and p.get("text"):
                    prefix = "[bold blue]用户[/]" if role == "user" else "[bold green]AI[/]"
                    lines.append(f"{prefix}：{p['text']}")
                elif ptype == "reasoning" and p.get("text"):
                    lines.append(f"[dim italic]推理：{p['text']}[/]")
                elif ptype == "tool":
                    st = p.get("state") or {}
                    lines.append(f"[dim]工具 {st.get('title', p.get('tool', '工具'))} {st.get('status', '')}[/]")
        self.call_from_thread(self.emit, "\n".join(lines))
        self.call_from_thread(self.emit, "[dim]—— 以上为历史，以下为新的对话 ——[/]")

    def on_question_result(self, result) -> None:
        """QuestionScreen 回答回调。"""
        if result is not None:
            self.send_answer(result)

    @work(thread=True)
    def start_project(self, title: str, kind: str) -> None:
        # 等服务就绪（表单已先弹出，这里最多等 95s）
        deadline = time.time() + 95
        while not self.services_ready:
            if time.time() > deadline:
                self.call_from_thread(self.emit, f"[red]服务未就绪：{self.service_error}[/]")
                return
            time.sleep(0.5)
        self.call_from_thread(self.set_status, "创建项目…")
        try:
            r = http_json("POST", SERVER_PORT, "/api/v1/chat/projects", {"title": title, "kind": kind})
            if r.get("code") != 0:
                self.call_from_thread(self.emit, f"[red]创建项目失败：{r}[/]")
                return
            pid = r["data"]["projectId"]
            s = http_json("POST", SERVER_PORT, "/api/v1/chat/sessions", {"title": title, "projectId": pid})
            cid = s["data"]["chatSessionId"]
            self.cid, self.pid = cid, pid
            self.call_from_thread(self.emit,
                f"[bold green]✓ 项目 {pid}（{kind}）→ 会话 {cid}[/]\n[dim]可以开始对话了[/]")
            self.call_from_thread(self.ui_ready, f"项目 {pid} · {kind} · 会话 {cid}")
        except Exception as e:
            self.call_from_thread(self.emit, f"[red]初始化失败：{e}[/]")

    def on_input_submitted(self, event: Input.Submitted) -> None:
        if event.input.id != "chatinput" or self.cid is None:
            return
        text = event.value.strip()
        event.input.value = ""
        if not text:
            return
        if text in ("/quit", "exit"):
            # 退出前中止当前会话——否则模型 agent 仍在后台跑（用户反馈：quit 后模型还在工作）。
            self.abort_turn()
            self.exit()
            return
        if text == "/abort":
            self.abort_turn()
            return
        if text == "/plans":
            self.show_plans()
            return
        self.emit(f"\n[bold blue]你> [/]{text}")
        self.run_turn(text)

    @work(thread=True)
    def abort_turn(self) -> None:
        """中止当前会话 turn（后端 cancel ctx，SSE 收 session.idle 结束）。"""
        if self.cid is None:
            return
        try:
            http_json("POST", SERVER_PORT, f"/api/v1/chat/sessions/{self.cid}/abort", timeout=10)
            self.call_from_thread(self.emit, "[yellow]⏹ 已发送中止请求…[/]")
        except Exception as e:
            self.call_from_thread(self.emit, f"[red]中止失败：{e}[/]")

    @work(thread=True)
    def run_turn(self, text: str) -> None:
        self.set_busy(True)
        try:
            http_json("POST", SERVER_PORT, f"/api/v1/chat/sessions/{self.cid}/messages", {"text": text}, timeout=10)
        except Exception as e:
            self.set_busy(False)
            self.call_from_thread(self.emit, f"[red]发消息失败：{e}[/]")
            return
        # 当前 assistant 段：正文/reasoning 分块缓冲
        cur = {"text": "", "reasoning": ""}
        try:
            for event, data in sse_frames(self.cid):
                self.record_frame(event, data)
                self.handle_frame(event, data, cur)
        finally:
            self.set_busy(False)  # turn 结束（session.idle）→ 禁用 abort 按钮

    def record_frame(self, event: str, data: dict) -> None:
        """帧三路分发：内存序列（轨迹源）+ JSONL 落盘 + 侧栏帧面板一行。"""
        if self.recorder is None:
            self.frames_path = frames_store.new_frames_path(self.cid or "session")
            self.recorder = frames_store.FrameRecorder(self.frames_path, cid=self.cid or "")
            self.call_from_thread(self.emit, f"[dim]帧流落盘：{self.frames_path}[/]")
        rec = self.recorder.record(event, data)
        self.frame_records.append(rec)
        self.call_from_thread(self.ui_frame_line, frames_store.frame_line(rec))

    def ui_frame_line(self, line: str) -> None:
        """帧面板追加一行（UI 线程执行；worker 经 call_from_thread 调度）。"""
        self.query_one("#sidebar", SideBar).append_frame_line(line)

    @work(thread=True)
    def run_replay(self, path: str) -> None:
        """离线回放：帧流 → 帧面板 + 消息视图 + 轨迹源（不连 server、不二次落盘）。"""
        try:
            records, summary = replay_mod.load_replay(path)
        except ValueError as e:
            self.call_from_thread(self.emit, f"[red]{e}[/]")
            self.call_from_thread(self.set_status, "回放失败")
            return
        cur = {"text": "", "reasoning": ""}
        count = replay_mod.feed_replay(
            records, lambda rec: self._replay_frame(rec, cur))
        self.call_from_thread(self.emit, f"[bold green]✓ {summary}[/]")
        self.call_from_thread(self.set_status, f"回放完成 · {count} 帧 · {path}")
        self.call_from_thread(self.action_frames)

    def _replay_frame(self, rec: dict, cur: dict) -> None:
        self.frame_records.append(rec)
        self.call_from_thread(self.ui_frame_line, frames_store.frame_line(rec))
        self.handle_frame(rec.get("event"), rec.get("data") or {}, cur)

    def set_busy(self, busy: bool) -> None:
        """运行中启用「中止」按钮（模型 turn 中可中断），空闲禁用。"""
        self.busy = busy
        try:
            self._apply_busy_btn()
        except RuntimeError:
            self.call_from_thread(self._apply_busy_btn)

    def _apply_busy_btn(self) -> None:
        """设置 abort 按钮可用态。"""
        self.query_one("#abortbtn", Button).disabled = not self.busy

    def handle_frame(self, event: str, data: dict, cur: dict) -> None:
        if event == "message.part.delta" and data.get("delta"):
            part = str(data.get("partID", ""))
            d = data["delta"]
            if "reasoning" in part:
                cur["reasoning"] += d
                self.call_from_thread(self.stream, part, d, "dim italic")
            else:
                cur["text"] += d
                self.call_from_thread(self.stream, part, d, "")
        elif event == "message.part.updated":
            part = data.get("part", {})
            ptype = part.get("type")
            if ptype == "reasoning":
                cur["reasoning"] = ""
            elif ptype == "text":
                cur["text"] = ""
            elif ptype == "tool":
                self.call_from_thread(self.render_tool, part)
        elif event == "subagent.status":
            self.call_from_thread(self.emit,
                f"\n[cyan]⊞ 子agent {data.get('subagentId','')} {data.get('status','')} {data.get('task','')}[/]")
        elif event == "question.ask":
            if self.replay_path:  # 回放不弹交互框（对照回归用），文本化呈现
                self.call_from_thread(self.emit,
                    f"\n[yellow]❓（回放）{data.get('question','')}[/]")
                return
            self.active_question_id = data.get("interruptId", "")
            self.call_from_thread(self.push_screen, QuestionScreen(data.get("question", "")), self.on_question_result)
        elif event == "session.error":
            self.call_from_thread(self.emit, f"\n[red]错误：{data}[/]")

    def render_tool(self, part: dict) -> None:
        st = part.get("state", {})
        name = st.get("title", part.get("tool", "工具"))
        status = st.get("status", "running")
        color = {"running": "yellow", "completed": "green", "error": "red"}.get(status, "white")
        icon = {"running": "◌", "completed": "✓", "error": "✗"}.get(status, "?")
        lines = [f"[{color}]┌─ {icon} {name}[/]"]
        if st.get("input"):
            lines.append(f"[dim]  入参: {st['input'][:200]}[/]")
        if st.get("output"):
            lines.append(f"[{color}]  输出: {st['output'][:400]}[/]")
        if st.get("error"):
            lines.append(f"[red]  错误: {st['error'][:400]}[/]")
        lines.append(f"[{color}]└─[/]")
        self.emit("\n".join(lines))

    @work(thread=True)
    def send_answer(self, answer: str) -> None:
        try:
            http_json("POST", SERVER_PORT, f"/api/v1/chat/sessions/{self.cid}/answer",
                      {"interruptId": self.active_question_id, "answer": answer}, timeout=10)
        except Exception as e:
            self.call_from_thread(self.emit, f"[red]回答提交失败：{e}[/]")

    @work(thread=True)
    def show_plans(self) -> None:
        try:
            r = http_json("GET", SERVER_PORT, f"/api/v1/projects/{self.pid}/plan_history", timeout=15)
            self.call_from_thread(self.emit, "[dim]" + json.dumps(r.get("data"), ensure_ascii=False, indent=2)[:2000] + "[/]")
        except Exception as e:
            self.call_from_thread(self.emit, f"[red]拉方案历史失败：{e}[/]")

    def emit(self, msg: str = "") -> None:
        """写一个完整段落（RichLog 风格，无 end 参数——段落整体追加）。"""
        msgs = self.query_one("#messages", VerticalScroll)
        if msg:
            msgs.mount(Static(msg, markup=True))
            msgs.scroll_end(animate=False)

    def stream(self, part_id: str, delta: str, style: str = "") -> None:
        """打字机增量：持续更新同一 part 的 Static（区分 reasoning/text）。"""
        msgs = self.query_one("#messages", VerticalScroll)
        if part_id not in self.streams:
            st = Static("", markup=True)
            msgs.mount(st)
            self.streams[part_id] = [st, ""]
            msgs.scroll_end(animate=False)
        st, acc = self.streams[part_id]
        acc += delta
        self.streams[part_id][1] = acc
        st.update(f"[{style}]{acc}[/]" if style else acc)
        msgs.scroll_end(animate=False)

    def set_status(self, text: str) -> None:
        self.query_one("#status", Static).update(text)

    def ui_ready(self, status_text: str) -> None:
        """初始化完成后的 UI 收尾（必须在 UI 线程执行）。"""
        self.set_status(status_text)
        inp = self.query_one("#chatinput", Input)
        inp.disabled = False
        inp.focus()


def main():
    ap = argparse.ArgumentParser(description="AI_IFC agent TUI 调试工具")
    ap.add_argument("--scripted", action="store_true",
                    help="scriptedModel 离线模式（拉起 server 时清空 llmAPIKey；默认真实 LLM）")
    ap.add_argument("--replay", metavar="FRAMES_JSONL",
                    help="离线回放已保存帧流（不连 server，对照回归）")
    args = ap.parse_args()
    if not sys.stdin.isatty():
        sys.exit("agent_tui.py 需要交互终端")
    AgentApp(scripted=args.scripted, replay_path=args.replay).run()


if __name__ == "__main__":
    main()
