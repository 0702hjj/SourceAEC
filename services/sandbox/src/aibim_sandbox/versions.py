# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Immutable version snapshots, stored per model under the viewer data dir.

Snapshots live at ``{VIEWER_DATA_DIR}/models/{id}/versions/v{n}.<ext>`` (n
starts at 1; ``ext`` is the service product extension — ifc / dxf). The
first save snapshots the original upload as ``v1``; every successful save
snapshots the newly saved file as ``v{n+1}``. Snapshot files are
append-only and written atomically (write ``dest + ".tmp"`` then
``os.replace``).

W-0048 T1：原 services/ifc 与 services/cad 的 versions.py 合一，扩展名
（``.ifc``/``.dxf``）参数化为 ``ext`` keyword；服务侧 app/versions.py
薄适配用 functools.partial 绑定。
"""

from __future__ import annotations

import os
import re
import shutil
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

VERSION_NAME_RE = re.compile(r"^v\d+$")


def version_file_re(ext: str) -> "re.Pattern[str]":
    """Snapshot filename pattern for a product extension (v{n}.<ext>)."""
    return re.compile(r"^v(\d+)\." + re.escape(ext) + r"$")


def versions_dir(data_dir: str, model_id: str) -> str:
    """Return the versions directory path for a model id."""
    return os.path.join(data_dir, "models", model_id, "versions")


def version_path(
    data_dir: str, model_id: str, version: str, *, ext: str
) -> Optional[str]:
    """Return the snapshot path for a version name, or None if invalid/missing."""
    if not VERSION_NAME_RE.match(version):
        return None
    path = os.path.join(versions_dir(data_dir, model_id), f"{version}.{ext}")
    return path if os.path.isfile(path) else None


def list_snapshots(directory: str, file_re: "re.Pattern[str]") -> List[Dict[str, Any]]:
    """List v{n} snapshot files in a directory as {"version", "createdAt"}, oldest first."""
    entries = []
    if os.path.isdir(directory):
        for name in os.listdir(directory):
            match = file_re.match(name)
            if match:
                path = os.path.join(directory, name)
                created = datetime.fromtimestamp(
                    os.path.getmtime(path), timezone.utc
                ).isoformat()
                entries.append((int(match.group(1)), f"v{match.group(1)}", created))
    entries.sort()
    return [{"version": name, "createdAt": created} for _, name, created in entries]


def list_versions(data_dir: str, model_id: str, *, ext: str) -> List[Dict[str, Any]]:
    """List snapshots as {"version": "v1", "createdAt": ...}, oldest first."""
    return list_snapshots(versions_dir(data_dir, model_id), version_file_re(ext))


def snapshot(data_dir: str, model_id: str, src_path: str, *, ext: str) -> str:
    """Atomically copy src_path to the next version snapshot; return its name."""
    directory = versions_dir(data_dir, model_id)
    os.makedirs(directory, exist_ok=True)
    existing = list_versions(data_dir, model_id, ext=ext)
    next_n = int(existing[-1]["version"][1:]) + 1 if existing else 1
    return snapshot_as(data_dir, model_id, src_path, f"v{next_n}", ext=ext)


def snapshot_as(
    data_dir: str, model_id: str, src_path: str, version: str, *, ext: str
) -> str:
    """Atomically copy src_path to an explicitly named snapshot; return the name.

    Used by script_versions.save to keep ``scripts/v{n}.py`` and
    ``versions/v{n}.<ext>`` in lockstep (n chosen by the caller).
    """
    if not VERSION_NAME_RE.match(version):
        raise ValueError(f"invalid version name: {version}")
    directory = versions_dir(data_dir, model_id)
    os.makedirs(directory, exist_ok=True)
    dest = os.path.join(directory, f"{version}.{ext}")
    tmp = dest + ".tmp"
    shutil.copyfile(src_path, tmp)
    os.replace(tmp, dest)
    return version
