# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_versions 薄适配：领域层门面 aibim_editapi.script_versions 绑定
.ifc（W-0057 T2）。

实现单点在 aibim_sandbox.script_versions（W-0048 T1；扩展名参数化 + cad
building_text sidecar 并入统一签名）；本模块用 functools.partial 绑定扩展
名再导出，调用点签名不变。
"""

from functools import partial

from aibim_editapi import script_versions as _shared
from aibim_editapi.script_versions import (
    SCRIPT_FILE_RE,
    list_scripts,
    load_script,
    script_path,
    scripts_dir,
)

__all__ = [
    "SCRIPT_FILE_RE",
    "list_scripts",
    "load_script",
    "save",
    "script_path",
    "scripts_dir",
]

save = partial(_shared.save, ext="ifc")
