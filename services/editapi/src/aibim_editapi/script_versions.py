# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_versions 门面（W-0057 T2）：单点实现仍在 aibim_sandbox.script_versions。

W-0048 T1 已合一（扩展名参数化 + cad building_text sidecar 并入统一签名）；
本模块把它纳入 aibim_editapi 领域层命名空间。服务侧 app/script_versions.py
薄 shim 绑定扩展名再导出（save 调用点签名不变）。
"""

from __future__ import annotations

from aibim_sandbox.script_versions import (
    SCRIPT_FILE_RE,
    list_scripts,
    load_script,
    save,
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
