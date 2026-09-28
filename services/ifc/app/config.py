# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""config 薄适配：共享 Settings/load_settings 在 aibim_editapi（W-0057 T2）。

本模块 = IFC 服务 profile 单点：端口/env 名、flows 目录锚点（skill 资产）、
runner 沙箱静态差异全部声明在 PROFILE 一处；``Settings`` / ``load_settings``
原样再导出（env 语义与收编前逐字段一致）。
"""

from __future__ import annotations

from pathlib import Path

from aibim_editapi.config import Settings
from aibim_editapi.config import load_settings as _load_settings
from aibim_editapi.profile import ServiceProfile

__all__ = ["PROFILE", "SERVICE_ROOT", "Settings", "load_settings"]

SERVICE_ROOT = Path(__file__).resolve().parent.parent  # services/ifc

PROFILE = ServiceProfile(
    name="ifc",
    ext="ifc",
    service_root=SERVICE_ROOT,
    port_env="EDIT_SERVICE_PORT",
    default_port=8100,
    max_models_env="EDIT_SERVICE_MAX_MODELS",
    diff_timeout_env="EDIT_SERVICE_DIFF_TIMEOUT_S",
    flows_dir_env="AIIFC_FLOWS_DIR",
    default_flows_dir="../../skills/aiifc/references/docs/flows",
    # ifc 无 drawlib 共享画法层：env 名与默认路径留空 → drawlib_dir 恒 ""。
    drawlib_dir_env="",
    drawlib_repo_rel_paths=(),
    # script_runner 沙箱静态差异（app/script_runner.py 经 sandbox_config 折成
    # CONFIG）：产物 out.ifc / flows 校验器 script_lib / 直跑无 inner-runner。
    temp_prefix="aiifc-run-",
    product_name="out.ifc",
    product_label="IFC",
    flows_module="script_lib",
    validate_prefix="aiifc-validate-",
    # T4 默认依赖集 = 存量脚本白嫖清单（flows script_lib import ifcopenshell；
    # design_review 系 flows 直接 import numpy）。
    default_deps=("ifcopenshell>=0.8", "numpy"),
    reset_state_inner_runner=False,
    # locate/edit-call 字段名（W-0057 T1）：guid → designKey hop，body/响应键
    # designKey（共享路由按取值取名；cad 为 key 直查同名）。
    locate_query_param="guid",
    locate_resp_field="designKey",
    edit_body_field="designKey",
)


def load_settings() -> Settings:
    """Build IFC Settings from env（语义单点在 aibim_editapi.config）。"""
    return _load_settings(PROFILE)
