# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Script staging endpoints: WPS-style stage/params/undo/redo/discard（W-0057 T1
单一源；原 services/ifc 与 services/cad 的 app/routes_script_staging.py 合一）。

- ``PUT /models/{id}/script`` — stage a script edit (full replace, or
  params-only via ``{"params": {...}}`` which rewrites the PARAMS block
  server-side). Undo/redo buffer, 10 steps.
- ``GET  /models/{id}/script`` — current staged script (or last saved base).
  404 for models that have no script.
- ``GET  /models/{id}/script/params`` — ast-extracted PARAMS (form feed).
- ``POST /models/{id}/script/undo|redo|discard`` — WPS-style navigation.

Staging helpers shared with the sibling route modules（``staging_of`` /
``staging_or_seed`` / ``verify_current_script``）live here as the single
definition point; run/versions/locate import them (W-0048 T3 split).

校验隔离：所有 ``raise HTTPException`` 住在 verify* 函数，由两侧
tests/test_verify_isolation.py 机器强制（本包无白名单）。

差异=注入/取值：产物扩展名经 profile.ext；契约校验器经 runner（服务绑定
CONFIG 的 script_runner 适配）。本模块**不使用** ``from __future__ import
annotations``——路由工厂模式里 FastAPI 经 get_type_hints 按__globals__求值
注解，工厂局部类（locate 的 EditCallBody）必须以即时求值注解注册。
"""

import os
import shutil
from typing import Any, Dict, Optional

from fastapi import APIRouter, HTTPException, Path, Request
from pydantic import BaseModel

from aibim_sandbox import script_params, script_staging
from aibim_sandbox import script_versions
from aibim_sandbox.route_common import MODEL_ID_PATTERN
from aibim_sandbox.route_common import model_lock
from aibim_sandbox.route_common import model_upload_path

from .profile import ServiceProfile

__all__ = [
    "ScriptBody",
    "build_staging_router",
    "bootstrap_path",
    "preserve_bootstrap",
    "staging_of",
    "staging_or_seed",
    "verify_current_script",
    "verify_extracted_params",
    "verify_params_target",
    "verify_params_text",
    "verify_redo_available",
    "verify_script_body",
    "verify_undo_available",
]


class ScriptBody(BaseModel):
    """Body of PUT /models/{id}/script: exactly one of script / params."""

    script: Optional[str] = None
    params: Optional[Dict[str, Any]] = None
    note: str = ""


def verify_script_body(body: ScriptBody) -> None:
    """stage_script 请求规则：script / params 恰好二选一。"""
    if (body.script is None) == (body.params is None):
        raise HTTPException(
            status_code=422, detail="exactly one of script / params required"
        )


def verify_params_target(staging: script_staging.ScriptStaging) -> str:
    """params 改写前置条件：必须存在当前脚本（满足则返回它）。"""
    current = staging.current()
    if current is None:
        raise HTTPException(status_code=409, detail="no script to update params on")
    return current


def verify_params_text(
    staging: script_staging.ScriptStaging, params: Dict[str, Any]
) -> str:
    """params-only 改写：前置检查 + PARAMS 块重写，重写错误统一翻译 422。"""
    try:
        return script_params.replace_params(verify_params_target(staging), params)
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc))


def verify_current_script(
    staging: script_staging.ScriptStaging, status_code: int
) -> str:
    """当前脚本必须存在（读端点 404 / 写端点 409，由调用方定语义）。"""
    current = staging.current()
    if current is None:
        raise HTTPException(status_code=status_code, detail="no script for model")
    return current


def verify_extracted_params(current: str) -> Dict[str, Any]:
    """PARAMS ast 提取错误的唯一 HTTP 翻译点（422）。"""
    try:
        return script_params.extract_params(current)
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc))


def verify_undo_available(staging: script_staging.ScriptStaging) -> None:
    if not staging.can_undo():
        raise HTTPException(status_code=409, detail="nothing to undo")


def verify_redo_available(staging: script_staging.ScriptStaging) -> None:
    if not staging.can_redo():
        raise HTTPException(status_code=409, detail="nothing to redo")


def staging_of(request: Request, model_id: str) -> script_staging.ScriptStaging:
    return request.app.state.script_staging.get(model_id)


def bootstrap_path(data_dir: str, model_id: str, ext: str) -> str:
    return os.path.join(data_dir, "models", model_id, f"bootstrap.{ext}")


def preserve_bootstrap(
    request: Request,
    model_id: str,
    staging: script_staging.ScriptStaging,
    ext: str,
) -> None:
    """plain 态模型首次暂存脚本时，把上传原件原子复制为 bootstrap.<ext>。

    必须发生在任何 run 覆盖 uploads 之前；已有大版本或 bootstrap 已存在则跳过
    （存量 script-backed 模型不补建）。调用方须持有模型锁。
    """
    data_dir = request.app.state.settings.data_dir
    bootstrap = bootstrap_path(data_dir, model_id, ext)
    if os.path.exists(bootstrap) or staging.current() is not None:
        return
    if script_versions.list_scripts(data_dir, model_id):
        return
    uploads = os.path.join(data_dir, "uploads", f"{model_id}.{ext}")
    os.makedirs(os.path.dirname(bootstrap), exist_ok=True)
    tmp = bootstrap + ".tmp"
    shutil.copyfile(uploads, tmp)
    os.replace(tmp, bootstrap)


def staging_or_seed(
    request: Request, model_id: str, ext: str
) -> script_staging.ScriptStaging:
    """Staging for read endpoints; empty staging + existing big versions → seed
    base from the newest big version (archived models have scripts/v{n}.py but
    no staging buffer, so GET /script would otherwise 404).

    Idempotent (seed_base is a no-op once a current script exists); the seed
    mutation runs under the per-model lock with a re-check.
    """
    staging = staging_of(request, model_id)
    if staging.current() is not None:
        return staging
    data_dir = request.app.state.settings.data_dir
    scripts = script_versions.list_scripts(data_dir, model_id)
    if not scripts:
        return staging
    with model_lock(request, model_id, ext):
        staging = staging_of(request, model_id)
        if staging.current() is None:
            latest = script_versions.load_script(
                data_dir, model_id, scripts[-1]["version"]
            )
            staging.seed_base(latest)
    return staging


def build_staging_router(profile: ServiceProfile, runner) -> APIRouter:
    """Build the staging router; ``runner`` is the service-bound script_runner
    adapter (module exposing validate_script_text，CONFIG 已按 profile 折好)。"""
    ext = profile.ext
    router = APIRouter()

    def verify_script_contract(request: Request, text: str) -> None:
        """脚本契约校验：validate_script_text 错误的唯一 HTTP 翻译点。"""
        errors = runner.validate_script_text(request.app.state.settings, text)
        if errors:
            raise HTTPException(
                status_code=422, detail="脚本契约校验失败: " + "; ".join(errors)
            )

    @router.get("/models/{id}/script")
    def get_script(
        request: Request, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        """Return the current script (staged state, or last saved base)."""
        model_upload_path(request, id, ext)
        staging = staging_or_seed(request, id, ext)
        current = verify_current_script(staging, 404)
        return {
            "modelId": id,
            "script": current,
            "staged": staging.staged_count(),
            "canUndo": staging.can_undo(),
            "canRedo": staging.can_redo(),
            "maxSteps": script_staging.MAX_STEPS,
        }

    @router.put("/models/{id}/script")
    def stage_script(
        request: Request,
        body: ScriptBody,
        id: str = Path(pattern=MODEL_ID_PATTERN),
    ) -> Dict[str, Any]:
        """Stage a script edit: full replace, or params-only PARAMS-block rewrite."""
        model_upload_path(request, id, ext)
        verify_script_body(body)
        with model_lock(request, id, ext):
            staging = staging_of(request, id)
            if body.script is not None:
                text = body.script
            else:
                text = verify_params_text(staging, body.params or {})
            verify_script_contract(request, text)
            preserve_bootstrap(request, id, staging, ext)
            staging.push(text)
            return {
                "modelId": id,
                "staged": staging.staged_count(),
                "canUndo": staging.can_undo(),
                "canRedo": staging.can_redo(),
            }

    @router.get("/models/{id}/script/params")
    def get_script_params(
        request: Request, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        """Return the current script's PARAMS dict (ast extraction, no execution)."""
        model_upload_path(request, id, ext)
        staging = staging_or_seed(request, id, ext)
        current = verify_current_script(staging, 404)
        return {"modelId": id, "params": verify_extracted_params(current)}

    @router.post("/models/{id}/script/undo")
    def undo_script(
        request: Request, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        with model_lock(request, id, ext):
            staging = staging_of(request, id)
            verify_undo_available(staging)
            staging.undo()
            return {
                "modelId": id,
                "script": staging.current(),
                "canRedo": staging.can_redo(),
            }

    @router.post("/models/{id}/script/redo")
    def redo_script(
        request: Request, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        with model_lock(request, id, ext):
            staging = staging_of(request, id)
            verify_redo_available(staging)
            staging.redo()
            return {
                "modelId": id,
                "script": staging.current(),
                "canUndo": staging.can_undo(),
            }

    @router.post("/models/{id}/script/discard")
    def discard_script(
        request: Request, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        """Throw staged edits away; back to the last saved big version. No version."""
        with model_lock(request, id, ext):
            staging = staging_of(request, id)
            dropped = staging.discard()
            return {"modelId": id, "discarded": dropped, "script": staging.current()}

    return router
