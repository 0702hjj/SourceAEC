# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""routes_diff 薄适配：共享路由 aibim_editapi.routes_diff（W-0057 T1 单一源）。

IFC 差异经 DiffDeps 注入：引擎 = app.diffing.compute_diff、物化 =
ifc_materialize.materialize_version（沙箱重建缺失快照）。原 IFC 存量违规
（_version_or_404 / _run_diff_with_timeout 内联 raise）随合一收拢为共享
verify* 形态——tests/test_verify_isolation.py 的 routes_diff.py 白名单项
同步清零（该测试自带的「收拢后必须同步删除」机制）。

``GET /models/{id}/versions`` 历来挂在 IFC diff 面（cad 挂在 scripts 面），
本模块装配 versions listing router，URL 不变。compute_diff / model_lock 经
lambda call-time 转发到 app.diffing / app.route_common 模块属性——tests
monkeypatch 的 seam（routes_diff.json / diffing.compute_diff /
route_common.model_lock）不变。
"""

import json  # tests/test_diff.py 经本模块锚定 monkeypatch json.dump

from aibim_editapi.deps import DiffDeps
from aibim_editapi.routes_diff import build_diff_router
from aibim_editapi.routes_script_versions import build_versions_listing_router

from . import diffing, ifc_materialize
from . import route_common
from .config import PROFILE

__all__ = ["router"]

DEPS = DiffDeps(
    compute_diff=lambda old, new: diffing.compute_diff(old, new),
    materialize_version=ifc_materialize.materialize_version,
    model_lock=lambda request, model_id: route_common.model_lock(request, model_id),
    model_upload_path=lambda request, model_id: route_common.model_upload_path(
        request, model_id
    ),
)

router = build_versions_listing_router(PROFILE)
router.include_router(build_diff_router(PROFILE, DEPS))
