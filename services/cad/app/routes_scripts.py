# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Script-as-source router assembly（W-0057 T1 薄适配）。

装配单点在 aibim_editapi.routes_scripts（staging/run/versions/locate 四域
factory）；本模块组装 CAD 的 ScriptsDeps——runner 用本服务绑定的
script_runner 适配，CAD 特有钩子（render.json 发布 / building.json
sidecar / buildingChanges）注入。CAD 的 locate 为 key 直查（共享缺省形态，
无注入项）。
"""

import os  # tests/test_render.py 经本模块锚定 monkeypatch os.replace

from fastapi import APIRouter

from aibim_editapi.deps import ScriptsDeps
from aibim_editapi.routes_scripts import build_scripts_router
from aibim_editapi.routes_script_versions import build_versions_listing_router

from . import building_diff, script_runner
from .config import PROFILE
from .routes_script_run import AFTER_RUN, COMPUTE_DIFF, SAVE_SIDECARS

__all__ = ["router", "os"]

DEPS = ScriptsDeps(
    runner=script_runner,
    compute_diff=COMPUTE_DIFF,
    after_run=AFTER_RUN,
    save_sidecars=SAVE_SIDECARS,
    building_diff=building_diff,
)

router = build_scripts_router(PROFILE, DEPS)
# CAD 的 GET /models/{id}/versions 历来挂在 scripts 面（ifc 挂在 diff 面）。
router.include_router(build_versions_listing_router(PROFILE))
