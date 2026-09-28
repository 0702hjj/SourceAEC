# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""routes_script_versions 薄适配：共享路由 aibim_editapi.routes_script_versions
（W-0057 T1 单一源）。IFC 不注入 building_diff → script/diff 响应无
buildingChanges 键（行为同收编前）；``GET /versions`` 列表路由在 IFC 挂在
routes_diff 面（app/routes_diff.py 装配），本模块不重复注册。"""

from aibim_editapi.routes_script_versions import (  # noqa: F401
    VERSION_NAME_PATTERN,
    build_script_versions_router,
    build_versions_listing_router,
    verify_script_version,
)
