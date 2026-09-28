# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""routes_diff 薄适配：共享路由 aibim_editapi.routes_diff（W-0057 T1 单一源）。

CAD 差异经 DiffDeps 注入：引擎 = app.dxf_diffing.compute_diff、物化 =
dxf_materialize.materialize_version（沙箱重建缺失快照）。compute_diff /
model_lock 经 lambda call-time 转发到 app.dxf_diffing / app.route_common
模块属性——tests monkeypatch 的 seam（routes_diff.json / dxf_diffing.
compute_diff / route_common.model_lock）不变。
"""

import json  # tests/test_diff.py 经本模块锚定 monkeypatch json.dump

from aibim_editapi.deps import DiffDeps
from aibim_editapi.routes_diff import build_diff_router

from . import dxf_diffing, dxf_materialize
from . import route_common
from .config import PROFILE

__all__ = ["router"]

DEPS = DiffDeps(
    compute_diff=lambda old, new: dxf_diffing.compute_diff(old, new),
    materialize_version=dxf_materialize.materialize_version,
    model_lock=lambda request, model_id: route_common.model_lock(request, model_id),
    model_upload_path=lambda request, model_id: route_common.model_upload_path(
        request, model_id
    ),
)

router = build_diff_router(PROFILE, DEPS)
