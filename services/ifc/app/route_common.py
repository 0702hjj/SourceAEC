# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""route_common 薄适配：领域层门面 aibim_editapi.route_common（W-0057 T2）。

单点实现自 W-0048 T1 起在 aibim_sandbox.route_common（editapi 门面再导出）；
本模块用 functools.partial 绑定 .ifc 扩展名再导出。model_lock 签名统一为
(request, model_id)，锁来源 request.app.state.model_locks（main.py 安装）。
"""

from functools import partial

from aibim_editapi.route_common import MODEL_ID_PATTERN
from aibim_editapi.route_common import model_lock as _model_lock
from aibim_editapi.route_common import model_upload_path as _model_upload_path

__all__ = ["MODEL_ID_PATTERN", "model_lock", "model_upload_path"]

model_upload_path = partial(_model_upload_path, ext="ifc")
model_lock = partial(_model_lock, ext="ifc")
