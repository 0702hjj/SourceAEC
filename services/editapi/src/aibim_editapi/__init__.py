# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""aibim_editapi — services/ifc 与 services/cad 的共享 REST 编辑面领域层。

W-0057 T2：两侧逐行镜像的领域基座收编到此包，服务侧 app/ 只留薄 shim
（profile 声明 + re-export，公开名不变）。

- ``profile``：ServiceProfile——服务差异的显式声明类型（数据差异=字段值、
  功能有无=空值/False，共享实现不写 target 分叉）
- ``config``：Settings + load_settings(profile)（env 名/默认值/路径锚点全进 profile）
- ``script_runner``：sandbox_config(profile) + ScriptRunner（aibim_sandbox.runner
  的服务适配层；drawlib/inner-runner 差异由 profile 显式声明）
- ``route_common`` / ``versions`` / ``script_versions``：W-0048 T1 已合一的
  单点实现的领域层门面（实现仍单点在 aibim_sandbox，此处不复制第二份）
- ``materialize``（W-0059）：历史大版本按需物化单一源（ext 参数化）——
  原两侧 app/ifc|dxf_materialize.py 成对复制收编于此，服务侧只留 partial
  shim 绑 ext + runner（call-time 解析，保测试 patch seam）
- ``deps`` / ``routes_*``（W-0057 T1）：六个共享面路由模块的 router
  factory——每模块 ``build_*_router(profile, deps)``，服务特有协作件
  （diff 引擎、run 钩子、locate guid hop…）经 ``ScriptsDeps``/``DiffDeps``
  注入，数据差异走 ServiceProfile 字段，共享代码零 if-target。
"""

from __future__ import annotations

__all__ = [
    "config",
    "deps",
    "materialize",
    "profile",
    "route_common",
    "routes_diff",
    "routes_script_locate",
    "routes_script_run",
    "routes_script_staging",
    "routes_script_versions",
    "routes_scripts",
    "script_runner",
    "script_versions",
    "versions",
]
