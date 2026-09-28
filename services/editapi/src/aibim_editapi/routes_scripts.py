# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Script-as-source router assembly（W-0057 T1 单一源；原两侧
app/routes_scripts.py 合一）。

The build script is the single source of truth for generated models. 端点按
域拆分在四个 sibling 模块（staging / run / versions / locate），经 factory
构建后在此装配——URL/method/response model 与收编前逐端点一致：

- ``routes_script_staging`` — staging CRUD + staging helpers（单点）。
- ``routes_script_run`` — run/save/rollback + run 链路单点 + 服务钩子注入。
- ``routes_script_versions`` — list/script-diff/staging-diff（cad 另附
  buildingChanges；``GET /versions`` 列表路由单独构建，装配点服务侧自选）。
- ``routes_script_locate`` — locate/edit-call + map envelope helpers。

Cross-module imports flow one way（staging ← versions ← run ← locate），no
helper is copied twice. All mutating endpoints hold the per-model lock.
"""

from fastapi import APIRouter

from .deps import ScriptsDeps
from .profile import ServiceProfile
from .routes_script_locate import build_locate_router
from .routes_script_run import build_run_router
from .routes_script_staging import build_staging_router
from .routes_script_versions import build_script_versions_router

__all__ = ["ScriptsDeps", "build_scripts_router"]


def build_scripts_router(profile: ServiceProfile, deps: ScriptsDeps) -> APIRouter:
    """Build the full script-as-source router（服务差异全部经 deps 注入）。"""
    router = APIRouter()
    router.include_router(build_staging_router(profile, deps.runner))
    router.include_router(build_run_router(profile, deps))
    router.include_router(build_script_versions_router(profile, deps))
    router.include_router(build_locate_router(profile, deps))
    return router
