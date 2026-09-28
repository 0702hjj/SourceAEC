# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""dxf_materialize 薄适配（W-0059）：实现单点在 aibim_editapi.materialize。

历史大版本按需物化——只物化最新大版本，旧快照经沙箱重跑 ``scripts/v{n}.py``
重建进 ``models/{id}/dxf_cache/v{n}.dxf``（LRU 加速缓存，非状态；inner runner
先 reset flows 状态，XDATA key 计数每次重跑归零，重建与原快照语义等价——比较
必须走 ``dxf_diffing.compute_diff``，禁字节等）。完整契约见共享模块 docstring。

本模块只绑 ``ext="dxf"`` 与 ``runner``（app.script_runner shim 模块对象，
call-time 解析 run_script——lazy_materialize 测试在 app.script_runner 上的
monkeypatch seam 不变）；``materialize_version`` 模块路径与签名不变
（routes_diff 的 DiffDeps 注入点零改动）。
"""

from functools import partial

from aibim_editapi import materialize as _shared

from . import script_runner

EXT = "dxf"

materialize_version = partial(
    _shared.materialize_version, ext=EXT, runner=script_runner
)
