# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""versions 门面（W-0057 T2）：单点实现仍在 aibim_sandbox.versions。

W-0048 T1 已合一（扩展名参数化为 ``ext`` keyword）；本模块把它纳入
aibim_editapi 领域层命名空间。服务侧 app/versions.py 薄 shim 绑定扩展名
再导出（versions.VERSION_FILE_RE 等模块属性 seam 不变）。
"""

from __future__ import annotations

from aibim_sandbox.versions import (
    VERSION_NAME_RE,
    list_snapshots,
    list_versions,
    snapshot,
    snapshot_as,
    version_file_re,
    version_path,
    versions_dir,
)

__all__ = [
    "VERSION_NAME_RE",
    "list_snapshots",
    "list_versions",
    "snapshot",
    "snapshot_as",
    "version_file_re",
    "version_path",
    "versions_dir",
]
