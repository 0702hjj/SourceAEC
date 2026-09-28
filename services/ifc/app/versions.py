# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""versions 薄适配：领域层门面 aibim_editapi.versions 绑定 .ifc（W-0057 T2）。

实现单点在 aibim_sandbox.versions（W-0048 T1，扩展名参数化）；本模块用
functools.partial 绑定扩展名再导出，调用点与测试 seam
（versions.VERSION_FILE_RE 等模块属性）不变。
"""

from functools import partial

from aibim_editapi import versions as _shared

EXT = "ifc"

VERSION_NAME_RE = _shared.VERSION_NAME_RE
VERSION_FILE_RE = _shared.version_file_re(EXT)
versions_dir = _shared.versions_dir
list_snapshots = _shared.list_snapshots
version_path = partial(_shared.version_path, ext=EXT)
list_versions = partial(_shared.list_versions, ext=EXT)
snapshot = partial(_shared.snapshot, ext=EXT)
snapshot_as = partial(_shared.snapshot_as, ext=EXT)
