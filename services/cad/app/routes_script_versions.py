# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""routes_script_versions 薄适配：共享路由 aibim_editapi.routes_script_versions
（W-0057 T1 单一源）。CAD 经 routes_scripts.DEPS 注入 building_diff →
script/diff 响应带 buildingChanges（两侧 building.json sidecar 皆在 →
字段级 diff，否则 null）；``GET /versions`` 列表路由的装配见
app/routes_scripts.py（CAD 挂在 scripts 面，URL 不变）。"""

from aibim_editapi.routes_script_versions import (  # noqa: F401
    VERSION_NAME_PATTERN,
    build_script_versions_router,
    build_versions_listing_router,
    verify_script_version,
)
