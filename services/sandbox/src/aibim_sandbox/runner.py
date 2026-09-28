# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""runner.py — script-as-source 构建脚本的沙箱执行（W-0048 T1 合一）。

合并原 services/ifc 与 services/cad 的 script_runner 主体；服务间差异
（产物名/临时前缀/inner-runner/flows 校验器/额外挂载）全部经
``RunSpec`` 参数化。

执行模型：构建脚本（契约：顶层 ``PARAMS`` 字面量 dict +
``build(params, out_path)`` 入口 + ``__main__`` 守卫）以子进程执行。
``config.inner_runner`` 非空时子进程入口是该内层 runner（cad：import
flows helper + reset_state 后 runpy 跑用户脚本），否则直跑
``python script.py out``。子进程解释器来自 uv 内容寻址环境（W-0048 T4，
``deps.ensure_env``：PEP 723 声明或服务默认集，宿主机解析、缓存复用），
不再是服务自身 venv 的 ``sys.executable``。

隔离层（backend 构造）+ 本模块的运行时闸：

- **static gate**：flows 校验器 ``validate_script_contract``（ast，不执行）
  拒绝不合契约脚本 → 422。
- both backends：wall-clock 超时杀进程组；stdout/stderr 分块读、累计超
  上限杀进程组；产物与 map sidecar **发布前**大小与 JSON 校验；进程级
  并发闸（满即 429）；stderr tail (2KB) on failure → 422。

发布原子性（W-0048 T0 裁决点①）：map sidecar 的 JSON 合法性也在发布
**前**校验——旧实现超限 map 发布前拒、非法 JSON map 发布后拒（产物已
落盘），失败原子性不一致；统一为「先校验后发布」，任何 422 都不留产物。
"""

from __future__ import annotations

import hashlib
import importlib
import json
import logging
import os
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time
from typing import List, Optional

from fastapi import HTTPException

from . import backend, deps
from .spec import (
    FSIZE_LIMIT_BYTES,
    OUTPUT_LIMIT_BYTES,
    PRODUCT_LIMIT_BYTES,
    RUN_CONCURRENCY,
    RunSpec,
)

logger = logging.getLogger(__name__)


def _int_env(name: str, default: int) -> int:
    """正整数 env 配置；缺失/非正/非法回退默认。"""
    raw = os.environ.get(name)
    if not raw:
        return default
    try:
        value = int(raw)
    except ValueError:
        logger.warning("忽略非法环境变量 %s=%r（回退默认 %d）", name, raw, default)
        return default
    return value if value > 0 else default


def script_hash(script_text: str) -> str:
    """sha256 hex of the exact script text — ScriptMap 信封的绑定键。

    发布侧（run_script）把它写进 map 信封；消费侧（locate/edit-call）用它
    比对 staging 当前脚本，不一致即视为 map 过期（行号不可信）。
    """
    return hashlib.sha256(script_text.encode("utf-8")).hexdigest()


def _load_flows_lib(spec: RunSpec):
    """Import the flows contract validator module (config.flows_module)."""
    if spec.flows_dir not in sys.path:
        sys.path.insert(0, spec.flows_dir)
    try:
        return importlib.import_module(spec.config.flows_module)
    except Exception as exc:  # pragma: no cover - env problem
        raise HTTPException(
            status_code=500, detail=f"load {spec.config.flows_module}: {exc}"
        )


def validate_script_text(spec: RunSpec, script_text: str) -> List[str]:
    """Static contract check (ast, no execution). Empty list = passes."""
    flows_lib = _load_flows_lib(spec)
    with tempfile.TemporaryDirectory(prefix=spec.config.validate_prefix) as workdir:
        path = os.path.join(workdir, "script.py")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(script_text)
        return flows_lib.validate_script_contract(path)


def _tail(data: bytes, limit: int) -> str:
    return data[-limit:].decode("utf-8", errors="replace")


class _OutputGuard:
    """分块泵 stdout/stderr：累计字节超 cap 置 exceeded（主循环据此杀进程组）。

    替代 ``communicate()`` 全量读入内存：脚本 stdout 泛洪不再撑爆父进程；
    stderr 只留尾（tail_bytes）供失败诊断。
    """

    def __init__(self, cap: int, tail_bytes: int = 2048) -> None:
        self.cap = cap
        self.tail_bytes = tail_bytes
        self.total = 0
        self.stderr_tail = bytearray()
        self.lock = threading.Lock()
        self.exceeded = threading.Event()

    def pump(self, stream, *, is_stderr: bool) -> None:
        try:
            while True:
                chunk = stream.read(65536)
                if not chunk:
                    return
                with self.lock:
                    self.total += len(chunk)
                    if is_stderr:
                        self.stderr_tail = (self.stderr_tail + chunk)[-self.tail_bytes:]
                    if self.total > self.cap:
                        self.exceeded.set()
        except (OSError, ValueError):  # 进程组被杀后管道关闭
            return


def _kill_group(proc: subprocess.Popen) -> None:
    """killpg 杀整个进程组：只杀直接子进程会让脚本 fork 出的孙进程成孤儿
    继续跑（M5 终审 I2）。"""
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        proc.kill()


_RUN_GATE: Optional[threading.Semaphore] = None
_RUN_GATE_LOCK = threading.Lock()


def _run_gate() -> threading.Semaphore:
    """进程级 run/save 并发闸（懒初始化；大小 SCRIPT_RUN_CONCURRENCY，默认 3）。

    每次 run 驻留 1 GiB rlimit + 最长 60s 子进程，不设闸会被并发请求拖垮
    整机。测试可 monkeypatch 模块级 ``_RUN_GATE`` 替换闸实例。
    """
    global _RUN_GATE
    with _RUN_GATE_LOCK:
        if _RUN_GATE is None:
            _RUN_GATE = threading.Semaphore(
                _int_env("SCRIPT_RUN_CONCURRENCY", RUN_CONCURRENCY)
            )
        return _RUN_GATE


def run_script(
    spec: RunSpec,
    script_text: str,
    out_path: str,
    *,
    map_out: Optional[str] = None,
    timeout: Optional[int] = None,
) -> None:
    """Validate + execute script_text, publishing the product to out_path.

    The ScriptMap sidecar (``<product>.map.json`` from the sandbox) is
    published atomically alongside — wrapped in a ``{"scriptHash", "map"}``
    envelope that binds the map to the exact script text — to ``map_out``
    when given, else next to out_path.

    Raises HTTPException(422) on contract violations, timeouts, non-zero
    exits, a missing/empty/oversize product or invalid map sidecar, or an
    unresolvable PEP 723 dependency declaration (W-0048 T4); nothing is
    written to out_path then (先校验后发布). HTTPException(503)
    when bwrap is unavailable (fail-closed, W-0048 T2: 生产单后端；rlimit
    仅测试可经 ``SANDBOX_BACKEND=rlimit`` 显式选择), when uv is missing,
    or when the network is unreachable for dependency resolution (T4);
    HTTPException(429) when the process-wide run gate is full.
    """
    backend.verify_sandbox_backend(spec)
    errors = validate_script_text(spec, script_text)
    if errors:
        raise HTTPException(
            status_code=422, detail="脚本契约校验失败: " + "; ".join(errors)
        )

    gate = _run_gate()
    if not gate.acquire(blocking=False):
        raise HTTPException(
            status_code=429,
            detail="沙箱执行并发已达上限（SCRIPT_RUN_CONCURRENCY），请稍后重试",
        )
    try:
        _run_in_sandbox(spec, script_text, out_path, map_out, timeout)
    finally:
        gate.release()


def _validate_map_sidecar(tmp_map: str, product_limit: int) -> dict:
    """发布前校验 map sidecar：大小上限 + 合法 JSON + 顶层 dict。返回 entries。"""
    if os.path.getsize(tmp_map) > product_limit:
        raise HTTPException(
            status_code=422,
            detail=f"脚本产出的 map sidecar 超过大小上限({product_limit}B)",
        )
    try:
        with open(tmp_map, encoding="utf-8") as fh:
            entries = json.load(fh)
    except (OSError, ValueError):
        raise HTTPException(
            status_code=422,
            detail="脚本产出的 map sidecar 不是合法 JSON",
        )
    if not isinstance(entries, dict):
        raise HTTPException(
            status_code=422,
            detail="脚本产出的 map sidecar 不是合法 JSON",
        )
    return entries


def _publish_map_envelope(
    entries: dict, script_text: str, map_dest: str
) -> None:
    """map 信封 {"scriptHash", "map"} 原子发布（tmp + os.replace）。"""
    envelope = json.dumps(
        {"scriptHash": script_hash(script_text), "map": entries},
        ensure_ascii=False,
    )
    dest_dir = os.path.dirname(map_dest)
    if dest_dir:
        os.makedirs(dest_dir, exist_ok=True)
    map_tmp = map_dest + ".tmp"
    with open(map_tmp, "w", encoding="utf-8") as fh:
        fh.write(envelope)
    os.replace(map_tmp, map_dest)


def _run_in_sandbox(
    spec: RunSpec,
    script_text: str,
    out_path: str,
    map_out: Optional[str],
    timeout: Optional[int],
) -> None:
    """run_script 的落盘+执行+校验+发布主体（并发闸由调用方持有）。"""
    cfg = spec.config
    if timeout is None:
        timeout = cfg.run_timeout_s
    # T4：PEP 723 声明或服务默认集 → uv 内容寻址环境（宿主机解析，缓存
    # 复用；422 声明非法/依赖不可解析，503 uv 缺失/断网）。
    env = deps.ensure_env(spec, script_text)
    with tempfile.TemporaryDirectory(prefix=cfg.temp_prefix) as workdir:
        script_path = os.path.join(workdir, "script.py")
        with open(script_path, "w", encoding="utf-8") as fh:
            fh.write(script_text)
        tmp_out = os.path.join(workdir, cfg.product_name)
        if cfg.inner_runner is not None:
            runner_path = os.path.join(workdir, "_runner.py")
            with open(runner_path, "w", encoding="utf-8") as fh:
                fh.write(cfg.inner_runner)
            cmd_tail = [env.python, runner_path, script_path, tmp_out]
        else:
            cmd_tail = [env.python, script_path, tmp_out]

        cmd = backend.sandbox_cmd(spec, workdir, extra_binds=env.ro_binds) + cmd_tail
        nproc = backend._nproc_budget(cfg.max_procs)  # 父进程算好，preexec 不扫 /proc
        fsize = _int_env("SCRIPT_MAX_FSIZE_BYTES", FSIZE_LIMIT_BYTES)
        proc = subprocess.Popen(
            cmd,
            cwd=workdir,
            env=backend.sandbox_env(spec, workdir),
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            preexec_fn=lambda: backend._limits(cfg, nproc, fsize),
            start_new_session=True,  # child 成进程组组长，超时杀整组
        )
        guard = _OutputGuard(
            _int_env("SCRIPT_MAX_OUTPUT_BYTES", OUTPUT_LIMIT_BYTES),
            tail_bytes=cfg.stderr_tail_bytes,
        )
        pumps = [
            threading.Thread(
                target=guard.pump, args=(proc.stdout,),
                kwargs={"is_stderr": False}, daemon=True,
            ),
            threading.Thread(
                target=guard.pump, args=(proc.stderr,),
                kwargs={"is_stderr": True}, daemon=True,
            ),
        ]
        for pump in pumps:
            pump.start()
        timed_out = False
        flooded = False
        deadline = time.monotonic() + timeout
        while proc.poll() is None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                timed_out = True
                break
            if guard.exceeded.wait(timeout=min(0.05, remaining)):
                flooded = True
                break
        if timed_out or flooded:
            _kill_group(proc)
        proc.wait()
        for pump in pumps:
            pump.join(timeout=5)

        stderr_tail = _tail(bytes(guard.stderr_tail), cfg.stderr_tail_bytes)
        if flooded:
            detail = "脚本输出超过上限,已终止进程组"
            if stderr_tail:
                detail += ": " + stderr_tail
            raise HTTPException(status_code=422, detail=detail)
        if timed_out:
            detail = f"脚本执行超时(>{timeout}s),已终止进程组"
            if stderr_tail:
                detail += ": " + stderr_tail
            raise HTTPException(status_code=422, detail=detail)

        if proc.returncode != 0:
            tail = stderr_tail or f"exit code {proc.returncode}"
            raise HTTPException(
                status_code=422, detail=f"脚本执行失败(exit {proc.returncode}): {tail}"
            )
        if not os.path.isfile(tmp_out) or os.path.getsize(tmp_out) == 0:
            raise HTTPException(
                status_code=422,
                detail=f"脚本未产出 {cfg.product_label}(build 必须写入 argv[1] 输出路径)",
            )

        # 发布前校验（先校验后发布，裁决点①）：产物与 map sidecar 的大小
        # 上限 + map JSON 合法性全部通过后才落盘，任何 422 都不留产物。
        # RLIMIT_FSIZE 是内核层硬闸，这里是发布语义层。
        product_limit = _int_env("SCRIPT_MAX_PRODUCT_BYTES", PRODUCT_LIMIT_BYTES)
        if os.path.getsize(tmp_out) > product_limit:
            raise HTTPException(
                status_code=422,
                detail=f"脚本产物超过大小上限({product_limit}B)",
            )
        tmp_map = tmp_out + ".map.json"
        map_entries: Optional[dict] = None
        if os.path.isfile(tmp_map):
            map_entries = _validate_map_sidecar(tmp_map, product_limit)

        dest_tmp = out_path + ".tmp"
        shutil.copyfile(tmp_out, dest_tmp)
        os.replace(dest_tmp, out_path)

        # ScriptMap sidecar 随产物一并原子发布；本次无 sidecar 时清掉旧文件，
        # 防止上一轮留下的 map 与新产物错位。信封 scriptHash 绑定生成它的
        # 那份脚本，消费侧按哈希比对 staging 以拒绝过期定位。
        map_dest = map_out if map_out is not None else out_path + ".map.json"
        if map_entries is not None:
            _publish_map_envelope(map_entries, script_text, map_dest)
        elif os.path.exists(map_dest):
            os.remove(map_dest)
