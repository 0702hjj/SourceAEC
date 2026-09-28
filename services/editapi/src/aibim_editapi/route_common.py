# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""route_common 门面（W-0057 T2）：单点实现仍在 aibim_sandbox.route_common。

W-0048 T1 已合一（MODEL_ID_PATTERN/model_upload_path/model_lock + ModelLocks）；
本模块把它纳入 aibim_editapi 领域层命名空间，作为 REST 编辑面共享包的统一
import 面（T1 路由 factory 同源取用）。服务侧 app/route_common.py 薄 shim
用 functools.partial 绑定产物扩展名再导出。
"""

from __future__ import annotations

from aibim_sandbox.route_common import (
    LOCKS_MAX,
    MODEL_ID_PATTERN,
    ModelLocks,
    model_lock,
    model_upload_path,
)

__all__ = [
    "LOCKS_MAX",
    "MODEL_ID_PATTERN",
    "ModelLocks",
    "model_lock",
    "model_upload_path",
]
