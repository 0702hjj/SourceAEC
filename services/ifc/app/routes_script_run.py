# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""routes_script_run 薄适配：共享路由 aibim_editapi.routes_script_run
（W-0057 T1 单一源）+ IFC run 链路服务特有钩子。

IFC 差异（全部经 ScriptsDeps 注入，共享代码零分叉）：

- ``after_run``：run 覆盖的是 pending entries 所附着的 IFC → 标记
  needs_replay 并卸载 registry 缓存（LRU on_evict 只在容量驱逐时触发）。
- ``alignment``：save 响应附 bootstrap.ifc（上传原件）vs 本次产物的构件级
  计数（语义单点在共享 bootstrap_alignment；计算失败降级 None，键仍在）。
- ``compute_diff`` 经 lambda 转发——tests monkeypatch 的是 app.diffing 模块
  属性（call-time 查找，patch seam 不变）。
"""

from functools import partial

from fastapi import Request

from aibim_editapi.routes_script_run import bootstrap_alignment

from . import diffing, script_runner
from .config import PROFILE

__all__ = ["AFTER_RUN", "COMPUTE_DIFF", "ALIGNMENT"]

COMPUTE_DIFF = lambda old, new: diffing.compute_diff(old, new)  # noqa: E731


def AFTER_RUN(request: Request, model_id: str, ifc_path: str) -> None:
    """run 后置：pending 标记 replay + registry 缓存卸载（IFC L1 遗产语义）。"""
    store = request.app.state.pending
    store._ensure(model_id)
    store.mark_needs_replay(model_id)
    request.app.state.registry.unload(ifc_path)


ALIGNMENT = partial(
    bootstrap_alignment, compute_diff=COMPUTE_DIFF, ext=PROFILE.ext
)
