# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""routes_script_locate 薄适配：共享路由 aibim_editapi.routes_script_locate
（W-0057 T1 单一源）。CAD 的 locate 查询参数即 XDATA key 本身（无 guid →
designKey hop、无 registry）——共享路由的 locate_key 缺省恒等即本服务形态，
无需注入，本模块仅作换装记录。"""

from aibim_editapi.routes_script_locate import build_locate_router  # noqa: F401
