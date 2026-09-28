# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Script locate/edit-call endpoints + current.map.json envelope helpers
（W-0057 T1 单一源；原两侧 app/routes_script_locate.py 合一）。

- ``GET  /models/{id}/script/locate?<guid|key>=`` — 构件定位 → CallSite
  (line/col/snippet/origin/params_keys)；staging 与 map 分叉 → 200 降级
  ``{"found": false, "stale": true}``（绝不跳错误行）。
- ``POST /models/{id}/script/edit-call`` — libcst 标量改写定位到的调用点
  实参，沙箱 run + staging.push；任何失败 422 零副作用，stale map → 409
  fail-closed，origin=traced → 422。

入参/出参字段名差异 = profile 取值（ifc：guid → designKey hop，响应/请求
键 designKey；cad：key 直查，同名）；guid→key 解析器经 ``deps.locate_key``
注入（ifc：registry 加载 + Pset AIIFC designKey；缺省：恒等 = cad 形态）。

map 信封 helper（``read_current_map`` / ``map_is_stale``）单点定义在本
模块；map 路径与 run 链路单点在 routes_script_run（W-0048 T3 split）。

校验隔离：所有 ``raise HTTPException`` 住在 verify* 函数。本模块**不使用**
``from __future__ import annotations``：工厂局部类（EditCallBody 按字段名
动态生成）必须以即时求值注解注册——FastAPI 经 get_type_hints 按
__globals__ 求值字符串注解，看不到工厂闭包局部名。
"""

import json
import os
from typing import Any, Callable, Dict, Optional, Tuple

from fastapi import APIRouter, HTTPException, Path, Query, Request
from pydantic import BaseModel, create_model

from aibim_sandbox import script_edit
from aibim_sandbox.route_common import MODEL_ID_PATTERN
from aibim_sandbox.route_common import model_lock, model_upload_path
from aibim_sandbox.runner import script_hash

from .deps import ScriptsDeps
from .profile import ServiceProfile
from .routes_script_run import current_map_path, make_run_into_uploads
from .routes_script_staging import (
    staging_of,
    staging_or_seed,
    verify_current_script,
)

__all__ = [
    "build_locate_router",
    "map_is_stale",
    "read_current_map",
    "verify_editable_origin",
    "verify_map_fresh",
    "verify_rewritten_script",
]


def verify_map_fresh(
    map_hash: Optional[str],
    entries: Optional[Dict[str, Any]],
    current: str,
    key: str,
) -> Dict[str, Any]:
    """edit-call 前置：map 缺失 → 404；staging 与 map 分叉 → 409 fail-closed。

    map 行号只对生成它的那份脚本有效，放行过期 map 会把改写落到错误的调用上。
    """
    if entries is None:
        raise HTTPException(status_code=404, detail=f"callsite not found: {key}")
    if map_is_stale(map_hash, current):
        raise HTTPException(
            status_code=409,
            detail="staging has un-run edits; run the script before edit-call",
        )
    return entries


def verify_editable_origin(entries: Dict[str, Any], key: str) -> Dict[str, Any]:
    """callsite 必须存在且可自动改写（origin=traced → 422，提示直接改脚本）。"""
    entry = entries.get(key)
    if entry is None:
        raise HTTPException(status_code=404, detail=f"callsite not found: {key}")
    if entry.get("origin") == "traced":
        raise HTTPException(
            status_code=422,
            detail="callsite not auto-editable (traced); edit the script directly",
        )
    return entry


def verify_rewritten_script(
    current: str, entry: Dict[str, Any], argument: str, value: Any
) -> str:
    """libcst 标量重写错误的唯一 HTTP 翻译点（422）。"""
    try:
        return script_edit.rewrite_call_argument(
            current, entry["line"], argument, value
        )
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc))


def read_current_map(map_path: str) -> Tuple[Optional[str], Optional[Dict[str, Any]]]:
    """读取 current.map.json 发布信封：返回 (script_hash, entries)。

    信封为 ``{"scriptHash": sha256(script), "map": {...}}``（run_script 发布）。
    文件缺失/损坏/非对象 → (None, None)；旧版裸 map（无信封）→ (None, {})，
    调用侧按过期处理（edit-call 409 / locate stale），绝不对形状变化 500。
    """
    if not os.path.isfile(map_path):
        return None, None
    try:
        with open(map_path, encoding="utf-8") as fh:
            raw = json.load(fh)
    except (OSError, ValueError):
        return None, None
    if not isinstance(raw, dict):
        return None, None
    script_hash_value = raw.get("scriptHash")
    entries = raw.get("map")
    if not isinstance(script_hash_value, str) or not isinstance(entries, dict):
        return None, {}
    return script_hash_value, entries


def map_is_stale(map_hash: Optional[str], current: Optional[str]) -> bool:
    """map 与 staging 当前脚本不同源（含无脚本可比对的旧版裸 map）→ 过期。"""
    if map_hash is None or current is None:
        return True
    return map_hash != script_hash(current)


def _edit_call_body_model(field: str) -> type:
    """按服务字段名生成 edit-call body 模型（ifc designKey / cad key）。"""
    return create_model(
        "EditCallBody",
        __doc__="Body of POST /models/{id}/script/edit-call: scalar argument rewrite.",
        **{field: (str, ...), "argument": (str, ...), "value": (Any, ...)},
    )


def build_locate_router(profile: ServiceProfile, deps: ScriptsDeps) -> APIRouter:
    """Build the locate/edit-call router（字段名差异 = profile 取值；
    guid→key hop = deps.locate_key 注入，缺省恒等 = cad 形态）。"""
    ext = profile.ext
    resp_field = profile.locate_resp_field
    EditCallBody = _edit_call_body_model(profile.edit_body_field)
    resolver: Callable = deps.locate_key or (lambda request, model_id, q: q)
    _run_into_uploads = make_run_into_uploads(deps, ext)
    router = APIRouter()

    @router.get("/models/{id}/script/locate")
    def locate_callsite(
        request: Request,
        q: str = Query(..., alias=profile.locate_query_param),
        id: str = Path(pattern=MODEL_ID_PATTERN),
    ) -> Dict[str, Any]:
        """Locate the script callsite for a 构件（查询值经服务解析器折为 design
        key——guid → designKey hop 或 key 直查）。

        解析不出 key（构件无 designKey）→ ``{"found": False}``；staging 与
        map 分叉（未 run 的暂存/undo/旧版裸 map）→ 200 降级
        ``{"found": false, "stale": true}``：行号不可信，绝不让前端跳到
        错误行。
        """
        model_upload_path(request, id, ext)
        key = resolver(request, id, q)
        if not key:
            # 解析不出 key（ifc 构件无 Pset designKey / 空查询值）→ 最小否定响应。
            return {"found": False}
        map_hash, entries = read_current_map(current_map_path(request, id))
        if entries is None:
            return {"found": False, resp_field: key}
        staging = staging_or_seed(request, id, ext)
        if map_is_stale(map_hash, staging.current()):
            return {"found": False, resp_field: key, "stale": True}
        entry = entries.get(key)
        if entry is None:
            return {"found": False, resp_field: key}
        return {"found": True, resp_field: key, **entry}

    @router.post("/models/{id}/script/edit-call")
    def edit_call(
        request: Request, body: EditCallBody, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        """Rewrite one scalar argument at a located callsite, then sandbox-run.

        顺序：定位 → 重写 → 契约校验+沙箱 run → staging.push；任何失败 422 零副作用。
        origin=traced 的调用点不可自动改写 → 422。
        staging 与 map 分叉（未 run 的暂存/undo/旧版裸 map）→ 409 fail-closed：
        map 行号只对生成它的那份脚本有效，放行会把改写落到错误的调用上。
        """
        model_upload_path(request, id, ext)
        key = getattr(body, profile.edit_body_field)
        with model_lock(request, id, ext):
            staging = staging_of(request, id)
            current = verify_current_script(staging, 409)
            map_hash, entries = read_current_map(current_map_path(request, id))
            entries = verify_map_fresh(map_hash, entries, current, key)
            entry = verify_editable_origin(entries, key)
            text = verify_rewritten_script(current, entry, body.argument, body.value)
            _run_into_uploads(request, id, text)
            staging.push(text)
            return {"modelId": id, "staged": staging.staged_count(), "script": text}

    return router
