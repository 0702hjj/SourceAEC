# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""ifc_materialize 薄适配（W-0059）：实现单点在 aibim_editapi.materialize。

历史大版本按需物化（spec §5.5, I5）——只物化最新大版本，旧快照经沙箱重跑
``scripts/v{n}.py`` 重建进 ``models/{id}/ifc_cache/v{n}.ifc``（LRU 加速缓存，
非状态；重建字节与原快照不同，比较必须走 ``diffing.compute_diff``，禁字节等）。
完整契约见共享模块 docstring。

本模块只绑 ``ext="ifc"`` 与 ``runner``（app.script_runner shim 模块对象，
call-time 解析 run_script——lazy_materialize 测试在 app.script_runner 上的
monkeypatch seam 不变）；``materialize_version`` 模块路径与签名不变
（routes_diff 的 DiffDeps 注入点零改动）。
"""

from functools import partial

from aibim_editapi import materialize as _shared

from . import script_runner

EXT = "ifc"

materialize_version = partial(
    _shared.materialize_version, ext=EXT, runner=script_runner
)
