# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Big-version snapshots for script-as-source models.

A big version is a designer-approved checkpoint. It stores the build script
(``{data_dir}/models/{id}/scripts/v{n}.py`` — the single source of truth).
Only the latest big version keeps its derived product snapshot
(``versions/v{n}.<ext>``); older snapshots with a rebuildable script are
deleted on save and regenerated on demand (lazy materialize). Only explicit
"save" points create big versions; in-between staging edits do not persist
and produce no version.

Script and product numbering stay in lockstep: ``n = max(next script n,
next product n)``, so a script version never collides with a plain snapshot
(other edit surfaces share ``versions/``). Snapshots without a script are
never pruned. A sidecar ``v{n}.meta.json`` holds the note/timestamp;
listings read the ``*.py`` files only.

Rollback = restore a saved script (re-run is downstream), never a per-step
revert.

W-0048 T1：原 services/ifc 与 services/cad 的 script_versions.py 合一——
扩展名参数化为 ``ext``；cad 的 ``building_text`` sidecar
（``scripts/v{n}.building.json``）并入统一签名（ifc 不传即无 sidecar）。
"""

from __future__ import annotations

import json
import os
import re
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

from . import versions

SCRIPT_FILE_RE = re.compile(r"^v(\d+)\.py$")


def scripts_dir(data_dir: str, model_id: str) -> str:
    return os.path.join(data_dir, "models", model_id, "scripts")


def script_path(data_dir: str, model_id: str, version: str) -> Optional[str]:
    if not versions.VERSION_NAME_RE.match(version):
        return None
    path = os.path.join(scripts_dir(data_dir, model_id), f"{version}.py")
    return path if os.path.isfile(path) else None


def _load_meta(data_dir: str, model_id: str, version: str) -> Dict[str, Any]:
    path = os.path.join(scripts_dir(data_dir, model_id), f"{version}.meta.json")
    if not os.path.isfile(path):
        return {}
    try:
        with open(path, "r", encoding="utf-8") as fh:
            return json.load(fh)
    except (OSError, ValueError):
        return {}


def list_scripts(data_dir: str, model_id: str) -> List[Dict[str, Any]]:
    """List script big versions as {"version", "createdAt", "note"}, oldest first."""
    entries = versions.list_snapshots(scripts_dir(data_dir, model_id), SCRIPT_FILE_RE)
    for entry in entries:
        entry["note"] = _load_meta(data_dir, model_id, entry["version"]).get("note", "")
    return entries


def load_script(data_dir: str, model_id: str, version: str) -> str:
    """Load a saved build script by version name (raise KeyError if missing)."""
    path = script_path(data_dir, model_id, version)
    if path is None:
        raise KeyError(f"script version not found: {version}")
    with open(path, "r", encoding="utf-8") as fh:
        return fh.read()


def _write_atomic(path: str, content: str) -> None:
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        fh.write(content)
    os.replace(tmp, path)


def _prune_rebuildable_snapshots(
    data_dir: str, model_id: str, keep_version: str, ext: str
) -> None:
    """Delete older ``versions/v{m}.<ext>`` snapshots that have a script.

    Only the latest big version stays materialized; versions with
    ``scripts/v{m}.py`` are rebuildable on demand (lazy materialize).
    Snapshots without a script are preserved.
    """
    keep_n = int(keep_version[1:])
    for m in range(1, keep_n):
        version = f"v{m}"
        if script_path(data_dir, model_id, version) is None:
            continue
        snapshot = versions.version_path(data_dir, model_id, version, ext=ext)
        if snapshot is not None:
            os.remove(snapshot)


def save(
    data_dir: str,
    model_id: str,
    script_text: str,
    src_path: str,
    note: str = "",
    map_text: Optional[str] = None,
    building_text: Optional[str] = None,
    *,
    ext: str,
) -> str:
    """Save a big version: script snapshot + product snapshot; return version name.

    Both snapshots use the same ``n`` (max of either side's next number), so
    ``scripts/v{n}.py`` and ``versions/v{n}.<ext>`` always pair up.
    ``map_text`` (the run's ScriptMap JSON) is snapshotted as
    ``scripts/v{n}.map.json`` in the same lockstep; scripts that produced no
    map get no map sidecar. ``building_text`` (deliverable building.json,
    cad C1) is snapshotted as ``scripts/v{n}.building.json`` in the same
    lockstep; absent → no sidecar. After the new snapshot lands, older
    rebuildable product snapshots are pruned (only the latest stays
    materialized).
    """
    directory = scripts_dir(data_dir, model_id)
    os.makedirs(directory, exist_ok=True)
    script_list = list_scripts(data_dir, model_id)
    product_list = versions.list_versions(data_dir, model_id, ext=ext)
    next_script = int(script_list[-1]["version"][1:]) + 1 if script_list else 1
    next_product = int(product_list[-1]["version"][1:]) + 1 if product_list else 1
    n = max(next_script, next_product)
    version = f"v{n}"

    _write_atomic(os.path.join(directory, f"{version}.py"), script_text)
    meta = {
        "version": version,
        "note": note,
        "savedAt": datetime.now(timezone.utc).isoformat(),
    }
    _write_atomic(
        os.path.join(directory, f"{version}.meta.json"),
        json.dumps(meta, ensure_ascii=False, indent=2),
    )
    if map_text is not None:
        _write_atomic(os.path.join(directory, f"{version}.map.json"), map_text)
    if building_text is not None:
        _write_atomic(os.path.join(directory, f"{version}.building.json"), building_text)
    versions.snapshot_as(data_dir, model_id, src_path, version, ext=ext)
    _prune_rebuildable_snapshots(data_dir, model_id, version, ext)
    return version
