# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Model diff endpoint（语义 diff；W-0057 T1 单一源，原两侧
app/routes_diff.py 合一——ifc 的存量违规形态顺带收拢为 cad 的 verify* 形态）。

``POST .../diff`` compares two snapshots (or a snapshot against the current
upload state) and returns the flat entity-key-keyed schema produced by the
injected engine（ifc diffing.compute_diff / cad dxf_diffing.compute_diff）。

Diff results between two immutable snapshots are cached next to them at
``versions/diff-{base}-{target}.json``. Diffs against ``target="current"``
are never cached: the uploads file is mutable, so there is no stable cache
key.

Historical big versions keep no materialized product（spec §5.5）：a missing
snapshot with a surviving script is rebuilt on demand into the LRU cache
（deps.materialize_version——ifc_materialize / dxf_materialize）；with neither
it is a 404. The per-model lock（deps.model_lock）is acquired by the executor
worker itself, covering rebuild + diff read: the LRU eviction's ``os.remove``
can never race a resolved cache path that ``compute_diff`` has not opened
yet, nor the cache-hit ``utime``. Holding it in the worker (not the handler)
also means a 504 timeout does not release it: the abandoned worker finishes
under the lock, so the next request serializes behind it instead of
reopening the concurrent-write window.

deps 的 callable 经属性调用（call-time 解析）：两侧测试把 monkeypatch 打在
``app.diffing`` / ``app.dxf_diffing`` / ``app.route_common`` 模块属性上，
服务侧 shim 以 lambda 转发注入即可保住既有 patch seam。
"""

import atexit
import json
import os
import threading
from concurrent.futures import ThreadPoolExecutor, TimeoutError as FutureTimeoutError
from typing import Any, Callable, Dict

from fastapi import APIRouter, HTTPException, Path, Request
from pydantic import BaseModel, Field

from aibim_sandbox.route_common import MODEL_ID_PATTERN
from aibim_sandbox.versions import versions_dir

from .config import Settings
from .deps import DiffDeps
from .profile import ServiceProfile
from .routes_script_versions import VERSION_NAME_PATTERN

__all__ = [
    "DIFF_TARGET_PATTERN",
    "DiffBody",
    "DiffDeps",
    "build_diff_router",
    "verify_diff_within_timeout",
    "verify_version_ref",
]

# diff 计算（引擎解析 + 沙箱重建）是 CPU 密集，阻塞在 sync handler 的
# 线程里会占死 FastAPI threadpool。用独立线程池执行，handler 侧 future.result(timeout)
# 超时即返回 504；残余 worker 继续跑完（per-model 锁由 worker 持有，见模块 docstring），
# handler 不等待也不释放锁。
_DIFF_EXECUTOR = ThreadPoolExecutor(max_workers=2, thread_name_prefix="diff")
atexit.register(_DIFF_EXECUTOR.shutdown, wait=False)

# target 在版本名之外额外放行字面量 "current"（对照当前 uploads 可变态，
# 永不缓存）；从 VERSION_NAME_PATTERN 派生，版本形状单点。
DIFF_TARGET_PATTERN = rf"^(?:{VERSION_NAME_PATTERN[1:-1]}|current)$"


class DiffBody(BaseModel):
    """Body of POST /models/{id}/diff. target also accepts "current".

    形状校验归声明式层（pydantic Field pattern，W-0038）：base/target 在
    handler 前被拦截——缓存路径 ``versions/diff-{base}-{target}.json`` 不在
    未校验输入上构造（注入路径分隔符无法越出预期目录）。
    """

    base: str = Field(..., pattern=VERSION_NAME_PATTERN)
    target: str = Field(..., pattern=DIFF_TARGET_PATTERN)


def verify_version_ref(
    deps: DiffDeps, settings: Settings, data_dir: str, model_id: str, version: str
) -> str:
    """版本引用必须可解析为产物路径（快照在 → 快照；缺失 → 沙箱重建进缓存）。

    满足则返回路径（派生数据）；快照与脚本皆无 → 404 的唯一翻译点。
    """
    try:
        return deps.materialize_version(data_dir, model_id, version, settings)
    except FileNotFoundError:
        raise HTTPException(status_code=404, detail=f"version not found: {version}")


def verify_diff_within_timeout(
    settings: Settings, compute: Callable[[], Dict[str, Any]]
) -> Dict[str, Any]:
    """diff 计算必须在 DIFF_TIMEOUT_S 内完成；超时 → 504 的唯一翻译点。

    残余线程继续跑完是接受语义（B4）：worker 全程持有 per-model 锁，
    超时返回 504 后残余 worker 仍在锁内安全完成物化/读取，不会重开并发写窗口；
    handler 不因它继续占用而阻塞返回，丢弃的只是 CPU 时间。

    饱和降级态（接受）：executor max_workers=2 时，两个超时 diff 各占一个 worker
    直至跑完；排队中的后续 diff 在 future.result(timeout) 内等不到空闲 worker
    → 必然 504，调用方应把 504 当「diff 未完成」重试。
    """
    future = _DIFF_EXECUTOR.submit(compute)
    try:
        return future.result(timeout=settings.diff_timeout_s)
    except FutureTimeoutError:
        raise HTTPException(status_code=504, detail="diff timed out")


def build_diff_router(profile: ServiceProfile, deps: DiffDeps) -> APIRouter:
    """Build the POST /diff router（引擎/物化/锁经 deps 注入，call-time 解析；
    profile 仅作工厂签名统一——ext 差异已折进 deps 的 ext 绑定 callable）。"""
    router = APIRouter()

    @router.post("/models/{id}/diff")
    def post_diff(
        request: Request, body: DiffBody, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        """Diff two model versions (or base version vs the current upload state)."""
        current_path = deps.model_upload_path(request, id)
        settings = request.app.state.settings
        data_dir = settings.data_dir

        cache_path = None
        if body.target != "current":
            cache_path = os.path.join(
                versions_dir(data_dir, id), f"diff-{body.base}-{body.target}.json"
            )
            if os.path.isfile(cache_path):
                with open(cache_path, "r", encoding="utf-8") as fh:
                    return json.load(fh)

        # 锁由 executor worker 持有（acquire/release 同在 worker 线程，RLock 语义合法）：
        # 覆盖物化+读取全程。超时返回 504 也不释放锁——残余 worker 继续锁内跑完，下一个
        # 请求的 worker 排队等锁 → 同模型并发物化/读写窗口关闭。
        def _compute_payload() -> Dict[str, Any]:
            with deps.model_lock(request, id):
                # 快照重建 + compute_diff 整体包进超时：materialize_version 也可能触发
                # 沙箱重跑脚本（CPU 密集），同样受 DIFF_TIMEOUT_S 约束。
                base_path = verify_version_ref(deps, settings, data_dir, id, body.base)
                if body.target == "current":
                    target_path = current_path
                else:
                    target_path = verify_version_ref(
                        deps, settings, data_dir, id, body.target
                    )
                return {
                    "base": body.base,
                    "target": body.target,
                    **deps.compute_diff(base_path, target_path),
                }

        payload = verify_diff_within_timeout(settings, _compute_payload)

        if cache_path is not None:
            # tmp 名必须按写者唯一：同 (base,target) 的并发请求都未命中结果缓存时，
            # 计算虽被模型锁串行，发布段却在锁外——共享 tmp 名会让一方的 os.replace
            # 把另一方在写的 tmp 改名，第二个 replace 抛 FileNotFoundError → 500
            # （W-0036/W-0037）。唯一 tmp + replace 后者覆盖前者，两次发布同 payload 等价。
            tmp = f"{cache_path}.{os.getpid()}.{threading.get_ident()}.tmp"
            with open(tmp, "w", encoding="utf-8") as fh:
                json.dump(payload, fh, ensure_ascii=False, indent=2)
            os.replace(tmp, cache_path)
        return payload

    return router
