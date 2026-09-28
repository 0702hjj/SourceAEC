# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""config 薄适配：共享 Settings/load_settings 在 aibim_editapi（W-0057 T2）。

本模块 = CAD 服务 profile 单点：端口/env 名、服务内 flows 目录、drawlib
共享画法层（archdxf + dxfkit 源目录，缺省自 skill **源**目录推导——不从
skills/dist/ 推导，CI 干净克隆无 dist 也能跑）、runner 沙箱静态差异全部
声明在 PROFILE 一处；``Settings`` / ``load_settings`` 原样再导出。
"""

from __future__ import annotations

from pathlib import Path

from aibim_editapi.config import Settings
from aibim_editapi.config import load_settings as _load_settings
from aibim_editapi.profile import ServiceProfile

__all__ = ["PROFILE", "SERVICE_ROOT", "Settings", "load_settings"]

SERVICE_ROOT = Path(__file__).resolve().parent.parent  # services/cad

PROFILE = ServiceProfile(
    name="cad",
    ext="dxf",
    service_root=SERVICE_ROOT,
    port_env="CAD_SERVICE_PORT",
    default_port=8200,
    max_models_env="CAD_SERVICE_MAX_MODELS",
    diff_timeout_env="CAD_SERVICE_DIFF_TIMEOUT_S",
    flows_dir_env="AIDXF_FLOWS_DIR",
    default_flows_dir="flows",
    # 共享画法层：archdxf + dxfkit 的 src 目录（冒号分隔，缺省自 repo 根下的
    # skill 源目录推导；单一事实源——skill editable install 与沙箱 PYTHONPATH
    # 引用同一份源码）。
    drawlib_dir_env="AIDXF_DRAWLIB_DIR",
    drawlib_repo_rel_paths=(
        "skills/aidxf/scripts/packages/archdxf/src",
        "skills/aidxf/scripts/packages/dxfkit/src",
    ),
    # script_runner 沙箱静态差异（app/script_runner.py 经 sandbox_config 折成
    # CONFIG）：产物 out.dxf / flows 校验器 cad_script_lib / inner-runner 包装。
    temp_prefix="aidxf-run-",
    product_name="out.dxf",
    product_label="DXF",
    flows_module="cad_script_lib",
    validate_prefix="aidxf-validate-",
    # T4 默认依赖集 = 存量脚本白嫖清单（ezdxf；archdxf/dxfkit 走 drawlib
    # 源目录 PYTHONPATH/ro-bind，不经 pip）。
    default_deps=("ezdxf>=1.3",),
    reset_state_inner_runner=True,
)


def load_settings() -> Settings:
    """Build CAD Settings from env（语义单点在 aibim_editapi.config）。"""
    return _load_settings(PROFILE)
