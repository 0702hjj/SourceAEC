# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Script big-version endpoints: version listing + script text diffs（W-0057 T1
单一源；原两侧 app/routes_script_versions.py 合一）。

- ``GET  /models/{id}/scripts`` — list big versions (scripts + snapshots).
- ``GET  /models/{id}/versions`` — materialized snapshot listing（ifc 挂在
  routes_diff、cad 挂在本模块——URL 两侧一致，装配点由服务侧 shim 自选）。
- ``POST /models/{id}/script/diff`` — unified text diff + PARAMS changes
  between two big versions；cad 响应额外携带 ``buildingChanges``
  （deps.building_diff 注入；ifc 不注入 → 无该键）。
- ``GET  /models/{id}/script/staging/diff`` — small-version diff between
  two staging steps.

``VERSION_NAME_PATTERN`` / ``verify_script_version`` 是大版本脚本的单点
定义，run（rollback）从此 import（W-0048 T3 split）。

校验隔离：所有 ``raise HTTPException`` 住在 verify* 函数，由两侧
tests/test_verify_isolation.py 机器强制（本包无白名单）。
"""

from typing import Any, Dict, Optional, Tuple

from fastapi import APIRouter, HTTPException, Path, Query, Request
from pydantic import BaseModel, Field

from aibim_sandbox import script_diff, script_staging, script_versions
from aibim_sandbox.route_common import MODEL_ID_PATTERN
from aibim_sandbox.route_common import model_upload_path
from aibim_sandbox.versions import list_versions

from .deps import ScriptsDeps
from .profile import ServiceProfile
from .routes_script_staging import staging_of

__all__ = [
    "ScriptDiffBody",
    "VERSION_NAME_PATTERN",
    "build_script_versions_router",
    "build_versions_listing_router",
    "verify_script_version",
    "verify_step_pair",
]

VERSION_NAME_PATTERN = r"^v\d+$"


class ScriptDiffBody(BaseModel):
    """Body of POST /models/{id}/script/diff: two big versions."""

    base: str = Field(..., pattern=VERSION_NAME_PATTERN)
    target: str = Field(..., pattern=VERSION_NAME_PATTERN)


def verify_script_version(data_dir: str, model_id: str, version: str) -> str:
    """大版本脚本必须存在（满足则返回脚本全文，404 的唯一翻译点）。"""
    try:
        return script_versions.load_script(data_dir, model_id, version)
    except KeyError as exc:
        raise HTTPException(status_code=404, detail=str(exc))


def verify_step_pair(
    staging: script_staging.ScriptStaging, from_: Optional[int], to: Optional[int]
) -> Tuple[int, int]:
    """staging diff 的步区间校验：至少两步（409），区间合法（422）。"""
    newest = staging.cursor
    if newest < 1:
        raise HTTPException(status_code=409, detail="fewer than two staged steps")
    i = newest - 1 if from_ is None else from_
    j = newest if to is None else to
    if not (0 <= i < j <= newest):
        raise HTTPException(
            status_code=422,
            detail=f"steps out of range: from={i} to={j} (valid 0..{newest}, from < to)",
        )
    return i, j


def _building_changes(deps: ScriptsDeps, data_dir: str, model_id: str, body: ScriptDiffBody):
    """cad：两侧 building.json sidecar 皆在 → 字段级 diff；否则 None。"""
    base_building = deps.building_diff.load_building_sidecar(
        data_dir, model_id, body.base
    )
    target_building = deps.building_diff.load_building_sidecar(
        data_dir, model_id, body.target
    )
    if base_building is not None and target_building is not None:
        return deps.building_diff.diff_building(base_building, target_building)
    return None


def build_script_versions_router(profile: ServiceProfile, deps: ScriptsDeps) -> APIRouter:
    """Build the script big-version router（cad 经 deps.building_diff 附
    buildingChanges；ifc 不注入 → 响应无该键）。"""
    ext = profile.ext
    router = APIRouter()

    @router.get("/models/{id}/scripts")
    def list_scripts(
        request: Request, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        """List script big versions (empty for upload-only models)."""
        model_upload_path(request, id, ext)
        data_dir = request.app.state.settings.data_dir
        return {
            "modelId": id,
            "scripts": script_versions.list_scripts(data_dir, id),
            "versions": list_versions(data_dir, id, ext=ext),
        }

    @router.post("/models/{id}/script/diff")
    def diff_script_versions(
        request: Request,
        body: ScriptDiffBody,
        id: str = Path(pattern=MODEL_ID_PATTERN),
    ) -> Dict[str, Any]:
        """Big-version script diff: unified text diff + PARAMS changes + stats."""
        model_upload_path(request, id, ext)
        data_dir = request.app.state.settings.data_dir
        base = verify_script_version(data_dir, id, body.base)
        target = verify_script_version(data_dir, id, body.target)
        payload: Dict[str, Any] = {
            "base": body.base,
            "target": body.target,
            "engine": "script",
            **script_diff.diff_scripts(base, target, body.base, body.target),
        }
        if deps.building_diff is not None:
            payload["buildingChanges"] = _building_changes(deps, data_dir, id, body)
        return payload

    @router.get("/models/{id}/script/staging/diff")
    def diff_staging_steps(
        request: Request,
        id: str = Path(pattern=MODEL_ID_PATTERN),
        from_: Optional[int] = Query(default=None, alias="from", ge=0),
        to: Optional[int] = Query(default=None, ge=0),
    ) -> Dict[str, Any]:
        """Small-version diff between two staging steps (default: the last two).

        Step indices address the staged states ``history[0..cursor]`` (0-based).
        Lightweight inline text diff + PARAMS changes; visible to both AI and user.
        """
        model_upload_path(request, id, ext)
        staging = staging_of(request, id)
        i, j = verify_step_pair(staging, from_, to)
        base, target = staging.history[i], staging.history[j]
        return {
            "from": i,
            "to": j,
            **script_diff.diff_scripts(base, target, f"step{i}", f"step{j}"),
        }

    return router


def build_versions_listing_router(profile: ServiceProfile) -> APIRouter:
    """``GET /models/{id}/versions``——物化快照列表（两侧同构 handler；ifc 挂在
    diff 面、cad 挂在 scripts 面，装配点差异在服务侧 shim，URL 不变）。"""
    ext = profile.ext
    router = APIRouter()

    @router.get("/models/{id}/versions")
    def get_versions(
        request: Request, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        """List materialized snapshots (empty + current=null before any save)."""
        model_upload_path(request, id, ext)
        data_dir = request.app.state.settings.data_dir
        listed = list_versions(data_dir, id, ext=ext)
        return {
            "versions": listed,
            "current": listed[-1]["version"] if listed else None,
        }

    return router
