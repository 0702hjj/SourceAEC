# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_runner 薄适配：共享适配层 aibim_editapi.script_runner（W-0057 T2）。

沙箱实现单点在 aibim_sandbox.runner（经 aibim_editapi 适配）；本模块从
config.PROFILE 折出 CAD 的 ``CONFIG``，再导出常量与入口——调用点与测试
seam（CONFIG/MAX_PROCS/FSIZE_LIMIT_BYTES/_spec/script_hash…）零改动。

CAD 差异（全部声明在 config.PROFILE）：产物 out.dxf / flows 校验器
cad_script_lib / inner-runner（reset_state + runpy 包装，编辑脚本确定性
XDATA key 计数）/ drawlib 折成 RunSpec 额外 PYTHONPATH/ro-binds。

测试 seam 纪律：monkeypatch 一律打在 aibim_sandbox.backend /
aibim_sandbox.runner 模块上——本模块不再别名这些函数，patch 在这里无效。
"""

from __future__ import annotations

from aibim_editapi import script_runner as _shared
from aibim_editapi.script_runner import (
    FSIZE_LIMIT_BYTES,
    MAX_PROCS,
    MEM_LIMIT_BYTES,
    OUTPUT_LIMIT_BYTES,
    PRODUCT_LIMIT_BYTES,
    RUN_CONCURRENCY,
    RUN_TIMEOUT_S,
    STDERR_TAIL_BYTES,
)

from .config import PROFILE

__all__ = [
    "CONFIG",
    "FSIZE_LIMIT_BYTES",
    "MAX_PROCS",
    "MEM_LIMIT_BYTES",
    "OUTPUT_LIMIT_BYTES",
    "PRODUCT_LIMIT_BYTES",
    "RUN_CONCURRENCY",
    "RUN_TIMEOUT_S",
    "STDERR_TAIL_BYTES",
    "run_script",
    "script_hash",
    "validate_script_text",
]

# CAD 沙箱静态差异的取值见 config.PROFILE（单点）；drawlib_dir 非空时由
# 共享适配层折成 RunSpec 额外 PYTHONPATH/ro-binds。
CONFIG = _shared.sandbox_config(PROFILE)

_adapter = _shared.ScriptRunner(CONFIG)
_spec = _adapter.spec
run_script = _adapter.run_script
validate_script_text = _adapter.validate_script_text
script_hash = _shared.script_hash
