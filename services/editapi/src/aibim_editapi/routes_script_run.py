# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Script run/save/rollback endpoints: sandboxed run + big-version snapshots
（W-0057 T1 单一源；原两侧 app/routes_script_run.py 合一）。

- ``POST /models/{id}/script/run`` — sandbox-run the staged script and
  atomically replace ``uploads/{id}.<ext>`` (no version). The response carries
  ``semanticDiff``: entity-level {added, removed, changed} counts vs the
  pre-run upload (null when the diff is unavailable).
- ``POST /models/{id}/script/save`` — sandbox-run, then snapshot
  ``scripts/v{n}.py`` + ``versions/v{n}.<ext>`` (atomic, lockstep). A failed
  run → 422 and no version.
- ``POST /models/{id}/script/rollback`` — restore a big version's script
  into staging, re-run it into uploads.

``run_into_uploads`` / ``current_map_path`` 是 run 链路的单点定义，locate
（edit-call）经 ``make_run_into_uploads`` 复用（W-0048 T3 split）。

漂移收敛（见任务报告漂移表）：两侧差异全部显式注入——``deps.after_run``
（ifc pending/registry 失效 vs cad render.json 发布）、``deps.save_sidecars``
（cad building.json sidecar）、``deps.alignment``（ifc save 响应对齐计数；
None = 响应无该键）、``deps.compute_diff``（ifcopenshell vs ezdxf 引擎）。
"""

import logging
import os
import shutil
import tempfile
from typing import Any, Callable, Dict, Optional

from fastapi import APIRouter, Path, Request
from pydantic import BaseModel, Field

from aibim_sandbox import script_versions
from aibim_sandbox.route_common import MODEL_ID_PATTERN
from aibim_sandbox.route_common import model_lock, model_upload_path

from .deps import ScriptsDeps
from .profile import ServiceProfile
from .routes_script_staging import bootstrap_path, staging_of, verify_current_script
from .routes_script_versions import VERSION_NAME_PATTERN, verify_script_version

logger = logging.getLogger(__name__)

__all__ = [
    "RollbackBody",
    "SaveBody",
    "bootstrap_alignment",
    "build_run_router",
    "current_map_path",
    "make_run_into_uploads",
    "remove_quiet",
    "run_diff_counts",
    "run_into_uploads",
    "snapshot_upload",
]


class SaveBody(BaseModel):
    """Optional body of POST /models/{id}/script/save."""

    note: str = ""


class RollbackBody(BaseModel):
    """Body of POST /models/{id}/script/rollback."""

    version: str = Field(..., pattern=VERSION_NAME_PATTERN)


def current_map_path(request: Request, model_id: str) -> str:
    return os.path.join(
        request.app.state.settings.data_dir, "models", model_id, "current.map.json"
    )


def remove_quiet(path: str) -> None:
    """Best-effort 删除：磁盘错误只记日志，绝不向上抛（原 cad 形态，两侧合一）。"""
    try:
        if os.path.exists(path):
            os.remove(path)
    except OSError:
        logger.warning("文件清理失败: %s", path, exc_info=True)


def run_into_uploads(
    request: Request,
    model_id: str,
    script: str,
    *,
    runner,
    ext: str,
    after_run: Optional[Callable],
) -> str:
    """Sandbox-run script into uploads/{id}.<ext>；服务特有钩子在 run 成功后触发。

    沙箱 run 发布 run's ScriptMap envelope 到 ``models/{id}/current.map.json``；
    ``after_run``（ifc：pending 标记 replay + registry 缓存卸载；cad：render.json
    发布；None：无钩子）以调用时属性解析，保证测试 patch seam。
    """
    product_path = model_upload_path(request, model_id, ext)
    runner.run_script(
        request.app.state.settings,
        script,
        product_path,
        map_out=current_map_path(request, model_id),
    )
    if after_run is not None:
        after_run(request, model_id, product_path)
    return product_path


def make_run_into_uploads(deps: ScriptsDeps, ext: str) -> Callable:
    """Bind (runner, ext, after_run) into the run 链路单点（locate 复用）。"""

    def _run_into_uploads(request: Request, model_id: str, script: str) -> str:
        return run_into_uploads(
            request,
            model_id,
            script,
            runner=deps.runner,
            ext=ext,
            after_run=deps.after_run,
        )

    return _run_into_uploads


def snapshot_upload(upload_path: str, ext: str) -> Optional[str]:
    """run 前把旧 uploads 产物复制到 tmp（供 run 后语义 diff 作基线）。

    复制失败（磁盘错误等）记日志返回 None——快照是观测增强，不阻断 run。
    调用方须持有模型锁（唯一写者）。
    """
    tmp: Optional[str] = None
    try:
        fd, tmp = tempfile.mkstemp(suffix=f".{ext}")
        os.close(fd)
        shutil.copyfile(upload_path, tmp)
        return tmp
    except OSError:
        logger.warning("run diff 旧产物快照失败: %s", upload_path, exc_info=True)
        if tmp is not None:
            remove_quiet(tmp)
        return None


def diff_counts(diff: Dict[str, Any]) -> Dict[str, int]:
    """语义 diff 结果 → {added, removed, changed} 计数（run/alignment 共用）。"""
    return {
        "added": len(diff["added"]),
        "removed": len(diff["removed"]),
        "changed": len(diff["changed"]),
    }


def run_diff_counts(
    old_snapshot: Optional[str],
    new_path: str,
    model_id: str,
    compute_diff: Callable,
) -> Optional[Dict[str, int]]:
    """旧产物快照 vs 本次 run 产物的构件级 diff 计数；失败降级 None。

    快照 tmp 文件无论成败都清理（单次使用的临时基线）。
    """
    if old_snapshot is None:
        return None
    try:
        diff = compute_diff(old_snapshot, new_path)
    except Exception:
        logger.exception("run semantic diff failed for model %s", model_id)
        return None
    finally:
        remove_quiet(old_snapshot)
    return diff_counts(diff)


def bootstrap_alignment(
    request: Request,
    model_id: str,
    product_path: str,
    *,
    compute_diff: Callable,
    ext: str,
) -> Optional[Dict[str, int]]:
    """bootstrap.<ext>（上传原件）vs 本次生成产物的语义 diff 计数（ifc §5.4）。

    版本已落盘，对齐计算失败绝不让 save 失败——记日志返回 None。
    """
    bootstrap = bootstrap_path(request.app.state.settings.data_dir, model_id, ext)
    if not os.path.isfile(bootstrap):
        return None
    try:
        return diff_counts(compute_diff(bootstrap, product_path))
    except Exception:
        logger.exception("bootstrap alignment diff failed for model %s", model_id)
        return None


def build_run_router(profile: ServiceProfile, deps: ScriptsDeps) -> APIRouter:
    """Build the run/save/rollback router（服务差异见 ScriptsDeps 注入项）。"""
    ext = profile.ext
    router = APIRouter()
    _run_into_uploads = make_run_into_uploads(deps, ext)

    @router.post("/models/{id}/script/run")
    def run_script_endpoint(
        request: Request, id: str = Path(pattern=MODEL_ID_PATTERN)
    ) -> Dict[str, Any]:
        """Sandbox-run the current staged script into uploads (preview; no version).

        响应附 ``semanticDiff``：旧 uploads 产物 vs 本次 run 产物的构件级
        {added, removed, changed} 计数（diff 失败/无旧产物降级 None，
        绝不让已成功的 run 失败）。
        """
        model_upload_path(request, id, ext)
        with model_lock(request, id, ext):
            current = verify_current_script(staging_of(request, id), 409)
            old_snapshot = snapshot_upload(model_upload_path(request, id, ext), ext)
            product_path = _run_into_uploads(request, id, current)
            return {
                "modelId": id,
                "ok": True,
                "semanticDiff": run_diff_counts(
                    old_snapshot, product_path, id, deps.compute_diff
                ),
            }

    @router.post("/models/{id}/script/save")
    def save_script(
        request: Request,
        id: str = Path(pattern=MODEL_ID_PATTERN),
        body: SaveBody | None = None,
    ) -> Dict[str, Any]:
        """Promote the staged script to a big version (run → snapshot script+product).

        A failed sandbox run → 422 and no version; staging is preserved so the
        script can be fixed and saved again. 服务可经钩子随版本快照额外交付
        sidecar（如 building.json）并在响应附对齐计数。
        """
        model_upload_path(request, id, ext)
        with model_lock(request, id, ext):
            staging = staging_of(request, id)
            current = verify_current_script(staging, 409)
            product_path = _run_into_uploads(request, id, current)
            note = body.note if body is not None else ""
            map_path = current_map_path(request, id)
            map_text: Optional[str] = None
            if os.path.isfile(map_path):
                with open(map_path, "r", encoding="utf-8") as fh:
                    map_text = fh.read()
            sidecars: Dict[str, str] = (
                deps.save_sidecars(request, id) if deps.save_sidecars is not None else {}
            )
            version = script_versions.save(
                request.app.state.settings.data_dir,
                id,
                current,
                product_path,
                note=note,
                map_text=map_text,
                ext=ext,
                **sidecars,
            )
            staging.save()
            payload: Dict[str, Any] = {"modelId": id, "version": version, "staged": 0}
            if deps.alignment is not None:
                payload["alignment"] = deps.alignment(request, id, product_path)
            return payload

    @router.post("/models/{id}/script/rollback")
    def rollback_script(
        request: Request,
        body: RollbackBody,
        id: str = Path(pattern=MODEL_ID_PATTERN),
    ) -> Dict[str, Any]:
        """Restore a big version's script into staging and re-run it into uploads."""
        model_upload_path(request, id, ext)
        with model_lock(request, id, ext):
            data_dir = request.app.state.settings.data_dir
            script = verify_script_version(data_dir, id, body.version)
            staging = request.app.state.script_staging.reset(id, base=script)
            _run_into_uploads(request, id, script)
            return {"modelId": id, "version": body.version, "script": staging.current()}

    return router
