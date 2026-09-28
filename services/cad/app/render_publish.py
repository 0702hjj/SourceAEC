# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""render.json 发布面（W-0059 自 render.py 拆出；W-0057 T1 时自
app/routes_script_run.py 迁入——CAD run 链路的 after_run 钩子实现，经
ScriptsDeps 注入共享路由）。

与 render.py（纯几何 payload 构建）按领域分层：本模块是带 request 依赖的
落盘 IO 面；best-effort 删除用共享 ``aibim_editapi.routes_script_run.remove_quiet``
（两侧合一，不再持有私有拷贝）。
"""

from __future__ import annotations

import json
import logging
import os

from fastapi import Request

from aibim_editapi.routes_script_run import remove_quiet

from . import render

logger = logging.getLogger(__name__)


def publish_render_json(request: Request, model_id: str, dxf_path: str) -> None:
    """run/save 后原子发布 render.json（tmp + os.replace，与 map sidecar 同纪律）。

    生成失败不阻断 run/save 主流程：记 warning 并删除旧 render.json，
    防止旧 payload 与新 uploads dxf 错位（map 侧车同纪律）。
    写盘失败（makedirs/open/os.replace）同样不阻断：记 warning 并尽量删旧，
    绝不让磁盘错误把一次成功的 run/save 变 500。
    调用方须持有模型锁（唯一写者，tmp 名无竞争）。
    """
    dest = os.path.join(
        request.app.state.settings.data_dir, "models", model_id, "render.json"
    )
    try:
        payload = render.build_render_payload(dxf_path)
    except Exception:
        logger.warning(
            "render.json 生成失败，删除旧文件防错位: model=%s", model_id,
            exc_info=True,
        )
        remove_quiet(dest)
        return
    try:
        os.makedirs(os.path.dirname(dest), exist_ok=True)
        tmp = dest + ".tmp"
        with open(tmp, "w", encoding="utf-8") as fh:
            json.dump(payload, fh, ensure_ascii=False)
        os.replace(tmp, dest)
    except OSError:
        logger.warning(
            "render.json 写盘失败，删除旧文件防错位: model=%s", model_id,
            exc_info=True,
        )
        remove_quiet(dest + ".tmp")
        remove_quiet(dest)
