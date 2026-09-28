#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""runtime.py —— agent TUI 的运行时底座（无 textual 依赖，可独立单测）。

从 agent_tui.py 拆出（W-0055）：
  - 端口/路径常量与 HTTP 工具（envelope 解析）；
  - 三服务探测与拉起（edit :8100 / cad :8200 / server :8090）；
  - scripted / 真实 LLM 模式：--scripted 时生成清空 llmAPIKey 的派生配置，
    以 `-config <派生配置>` 拉起 server（server 侧 env 只能加不能清，
    见 cmd/server/main.go loadConfig：空 env 不覆盖 json 值）；
  - SSE 流逐帧产出（event, sse_id, data），供 TUI 渲染 + frames_store 落盘。
"""
import json
import os
import shutil
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]  # tools/agent → 仓库根
DATA_DIR = os.environ.get("VIEWER_DATA_DIR", str(REPO / "data"))
SERVER_PORT = int(os.environ.get("VIEWER_SERVER_PORT", "8090"))
EDIT_PORT = int(os.environ.get("VIEWER_EDIT_PORT", "8100"))
CAD_PORT = int(os.environ.get("VIEWER_CAD_PORT", "8200"))
LOG_DIR = Path(os.environ.get("TMPDIR", "/tmp"))

AGENT_CACHE_DIR = Path(os.environ.get(
    "AIIFC_AGENT_CACHE", os.path.join(os.path.expanduser("~"), ".cache", "aiifc-agent")))


def url(port, path):
    return f"http://127.0.0.1:{port}{path}"


def http_json(method, port, path, body=None, timeout=60):
    req = urllib.request.Request(url(port, path), method=method)
    req.add_header("Content-Type", "application/json")
    data = None if body is None else json.dumps(body).encode("utf-8")
    with urllib.request.urlopen(req, data, timeout=timeout) as r:
        return json.loads(r.read().decode("utf-8"))


def is_alive(port, path="/health"):
    try:
        with urllib.request.urlopen(url(port, path), timeout=1):
            return True
    except urllib.error.HTTPError:
        return True
    except OSError:
        return False


def launch(name, cwd, cmd):
    env = os.environ.copy()
    # 共享 VIEWER_DATA_DIR（AGENTS 硬规则：edit-service/cad 与 Go server 必须同一 data 绝对路径，
    # 否则 stage/run script 时 edit-service 找不到 Go 写的 uploads/{id} 文件 → 404 model not found）
    env["VIEWER_DATA_DIR"] = DATA_DIR
    with open(LOG_DIR / f"agent_demo_{name}.log", "a") as f:
        subprocess.Popen(cmd, cwd=cwd, stdout=f, stderr=subprocess.STDOUT,
                         stdin=subprocess.DEVNULL, start_new_session=True, env=env)


# ---- scripted / 真实 LLM 模式 ------------------------------------------------

def read_server_config():
    """读 server 配置基座：server_config.json（本地敏感，gitignored）缺则 example 兜底。"""
    for name in ("server_config.json", "server_config.example.json"):
        p = REPO / "server" / name
        try:
            return json.loads(p.read_text()), name
        except (OSError, ValueError):
            continue
    return None, ""


def detect_llm_mode():
    """探明生效的 LLM 模式（与 cmd/server/main.go 同优先级：env 非空 > json）。

    返回 ("scripted" | "llm", 说明文本)。注意：只能推断由本工具拉起/本机配置的
    server——已运行的进程不受 --scripted 影响（README 有说明）。
    """
    key = os.environ.get("VIEWER_LLM_API_KEY", "")
    model = os.environ.get("VIEWER_LLM_MODEL", "")
    src = "env"
    if not key:
        cfg, name = read_server_config()
        if cfg is not None:
            key = cfg.get("llmAPIKey") or ""
            model = cfg.get("llmModel") or model
            src = name
    if not key:
        return "scripted", "llmAPIKey 为空 → scriptedModel 离线（确定性 mock）"
    return "llm", f"LLM 已配置（model={model or '默认'}，来源 {src}）"


def build_scripted_config(dest=None):
    """生成清空 llmAPIKey 的派生配置（--scripted 用）。

    基座 = server/server_config.json（缺则 example），只清 llmAPIKey，
    其余字段原样保留（dataDir 等相对路径以 server cwd 解析，不受 -config 绝对路径影响）。
    返回写入的路径；读不到基座配置返回 None。
    """
    cfg, _ = read_server_config()
    if cfg is None:
        return None
    cfg["llmAPIKey"] = ""
    dest = Path(dest) if dest else AGENT_CACHE_DIR / "scripted_server_config.json"
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(json.dumps(cfg, ensure_ascii=False, indent=2))
    return dest


def ensure_services(scripted=False):
    """探测并拉起三服务。scripted=True 时由本函数拉起的 server 用清空 key 的派生配置。

    已在跑的 server 不受 scripted 影响（返回 warning 文本，由调用方提示用户）。
    """
    warning = ""
    if not is_alive(EDIT_PORT):
        launch("edit", REPO / "services/ifc",
               ["uv", "run", "uvicorn", "app.main:app", "--host", "127.0.0.1", "--port", str(EDIT_PORT)])
    if not is_alive(CAD_PORT):
        launch("cad", REPO / "services/cad",
               ["uv", "run", "uvicorn", "app.main:app", "--host", "127.0.0.1", "--port", str(CAD_PORT)])
    if not is_alive(SERVER_PORT, "/api/v1/chat/sessions"):
        server_cmd = ["go", "run", "./cmd/server"]
        if scripted:
            cfg_path = build_scripted_config()
            if cfg_path is None:
                raise RuntimeError("读不到 server 配置基座，无法生成 scripted 派生配置")
            server_cmd += ["-config", str(cfg_path)]
        launch("server", REPO / "server", server_cmd)
    elif scripted:
        mode, detail = detect_llm_mode()
        if mode == "llm":
            warning = f"--scripted 仅影响本工具拉起的 server：:8090 已在跑且为真实 LLM（{detail}）"
    deadline = time.time() + 90
    while not is_alive(SERVER_PORT, "/api/v1/chat/sessions"):
        if time.time() > deadline:
            raise RuntimeError(f"server :{SERVER_PORT} 90s 内未就绪，看 {LOG_DIR}/agent_demo_server.log")
        time.sleep(2)
    return warning


# ---- SSE 流 ------------------------------------------------------------------

def parse_sse_lines(lines):
    """纯函数：SSE 文本行 → (event, sse_id, data) 帧三元组。

    遵循 SSE 语法（chat_sse.go 帧格式：id:/event:/data:，空行分帧）；
    data 非 JSON 时包 {"raw": ...}。测试与网络流共用（同 sse_frames）。
    """
    event, sid, data_lines = None, None, []
    for line in lines:
        line = line.rstrip("\r\n")
        if line == "":
            if event is not None or data_lines:
                try:
                    data = json.loads("\n".join(data_lines)) if data_lines else {}
                except ValueError:
                    data = {"raw": "\n".join(data_lines)}
                yield event, sid, data
            event, sid, data_lines = None, None, []
        elif line.startswith("id:"):
            raw = line[len("id:"):].strip()
            try:
                sid = int(raw)
            except ValueError:
                sid = None
        elif line.startswith("event:"):
            event = line[len("event:"):].strip()
        elif line.startswith("data:"):
            data_lines.append(line[len("data:"):].strip())
    if event is not None or data_lines:  # 输入结束冲刷尾帧（无结尾空行的健壮性）
        try:
            data = json.loads("\n".join(data_lines)) if data_lines else {}
        except ValueError:
            data = {"raw": "\n".join(data_lines)}
        yield event, sid, data


def sse_frames(cid):
    """会话 SSE 帧生成器：(event, sse_id, data)；session.idle/error 后停止。"""
    req = urllib.request.Request(url(SERVER_PORT, f"/api/v1/chat/sessions/{cid}/events"))
    with urllib.request.urlopen(req, timeout=600) as r:
        for frame in parse_sse_lines(_decode_lines(r)):
            event, _, data = frame
            yield event, data


def _decode_lines(resp):
    while True:
        line = resp.readline()
        if not line:
            break
        yield line.decode("utf-8", "replace")
