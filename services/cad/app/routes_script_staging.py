# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""routes_script_staging 薄适配：共享路由 aibim_editapi.routes_script_staging
（W-0057 T1 单一源）。端点注册进 routes_scripts 装配（app/routes_scripts.py），
本模块只再导出共享 helper（verify*/staging_of 等）供兼容引用。"""

from aibim_editapi.routes_script_staging import (  # noqa: F401
    build_staging_router,
    staging_of,
    staging_or_seed,
    verify_current_script,
    verify_extracted_params,
    verify_params_target,
    verify_params_text,
    verify_script_body,
)
