# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Script-as-source router assembly（W-0057 T1 薄适配）。

装配单点在 aibim_editapi.routes_scripts（staging/run/versions/locate 四域
factory）；本模块组装 IFC 的 ScriptsDeps——runner 用本服务绑定的
script_runner 适配，IFC 特有钩子来自 routes_script_run / routes_script_locate。
"""

from fastapi import APIRouter

from aibim_editapi.deps import ScriptsDeps
from aibim_editapi.routes_scripts import build_scripts_router

from . import script_runner
from .config import PROFILE
from .routes_script_locate import LOCATE_KEY
from .routes_script_run import AFTER_RUN, ALIGNMENT, COMPUTE_DIFF

__all__ = ["router"]

DEPS = ScriptsDeps(
    runner=script_runner,
    compute_diff=COMPUTE_DIFF,
    after_run=AFTER_RUN,
    alignment=ALIGNMENT,
    locate_key=LOCATE_KEY,
)

router = build_scripts_router(PROFILE, DEPS)
