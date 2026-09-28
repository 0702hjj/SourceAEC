#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""test_runtime.py —— LLM 模式检测 / scripted 派生配置 单测（无网络、无 textual）。"""
import json

import runtime
from runtime import build_scripted_config, detect_llm_mode

BASE_CFG = {
    "host": "127.0.0.1", "port": 8090, "dataDir": "../data",
    "editServiceURL": "http://127.0.0.1:8100", "cadServiceURL": "http://127.0.0.1:8200",
    "llmAPIKey": "sk-secret", "llmBaseURL": "https://api.example.com/v1",
    "llmModel": "gpt-x", "skillsDir": "../skills/dist",
}


# ---- detect_llm_mode（优先级：env 非空 > json） -------------------------------

def test_detect_env_key_wins(monkeypatch):
    monkeypatch.setenv("VIEWER_LLM_API_KEY", "sk-env")
    monkeypatch.setattr(runtime, "read_server_config", lambda: (dict(BASE_CFG), "server_config.json"))
    mode, detail = detect_llm_mode()
    assert mode == "llm" and "env" in detail


def test_detect_json_key_when_env_empty(monkeypatch):
    monkeypatch.delenv("VIEWER_LLM_API_KEY", raising=False)
    monkeypatch.setattr(runtime, "read_server_config", lambda: (dict(BASE_CFG), "server_config.json"))
    mode, detail = detect_llm_mode()
    assert mode == "llm" and "gpt-x" in detail and "server_config.json" in detail


def test_detect_scripted_when_both_empty(monkeypatch):
    monkeypatch.delenv("VIEWER_LLM_API_KEY", raising=False)
    cfg = dict(BASE_CFG, llmAPIKey="")
    monkeypatch.setattr(runtime, "read_server_config", lambda: (cfg, "server_config.example.json"))
    mode, detail = detect_llm_mode()
    assert mode == "scripted" and "scriptedModel" in detail


def test_detect_scripted_when_no_config(monkeypatch):
    monkeypatch.delenv("VIEWER_LLM_API_KEY", raising=False)
    monkeypatch.setattr(runtime, "read_server_config", lambda: (None, ""))
    assert detect_llm_mode()[0] == "scripted"


# ---- build_scripted_config（--scripted 的实现核心） ---------------------------

def test_build_scripted_config_clears_only_key(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime, "read_server_config", lambda: (dict(BASE_CFG), "server_config.json"))
    dest = tmp_path / "scripted_server_config.json"
    path = build_scripted_config(dest)
    assert path == dest
    cfg = json.loads(dest.read_text())
    assert cfg["llmAPIKey"] == ""                       # 只清 key
    assert cfg["llmModel"] == "gpt-x"                   # 其余原样保留
    assert cfg["dataDir"] == "../data"                  # 相对路径不动（server cwd 解析）
    assert set(cfg) == set(BASE_CFG)


def test_build_scripted_config_no_base_returns_none(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime, "read_server_config", lambda: (None, ""))
    assert build_scripted_config(tmp_path / "x.json") is None


def test_build_scripted_config_default_location_writable(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime, "read_server_config", lambda: (dict(BASE_CFG), "server_config.json"))
    monkeypatch.setattr(runtime, "AGENT_CACHE_DIR", tmp_path)
    path = build_scripted_config()
    assert path == tmp_path / "scripted_server_config.json"
    assert json.loads(path.read_text())["llmAPIKey"] == ""
