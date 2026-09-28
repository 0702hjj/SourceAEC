# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script-as-source 共享路由的服务特有协作件（W-0057 T1）。

共享路由（staging/run/versions/locate）对服务差异只认这里的注入项——
「差异=注入」，共享代码零 ``if target == ...`` 分叉。能力有无（after_run
钩子 / buildingChanges / alignment）用 ``None`` 显式声明缺省，与 profile
的空值纪律同构。

callable 一律在**调用时**解析目标属性（服务侧 shim 用 ``lambda a, b:
diffing.compute_diff(a, b)`` 形态传入）——两侧测试 monkeypatch 的是
``app.diffing`` / ``app.dxf_diffing`` / ``app.route_common`` 模块属性，
call-time 查找保证既有 patch seam 零改动。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable, Dict, Optional

from fastapi import Request

# 语义 diff 引擎：(旧产物路径, 新产物路径) -> {"added": [...], "removed": [...],
# "changed": [...]}（ifc diffing.compute_diff / cad dxf_diffing.compute_diff）。
ComputeDiff = Callable[[str, str], Dict[str, Any]]

# run/save 成功落盘后的服务钩子：(request, model_id, 产物绝对路径) -> None。
# ifc：pending 标记 needs_replay + registry 缓存卸载；cad：render.json 发布。
AfterRun = Callable[[Request, str, str], None]

# save 时随大版本 lockstep 落盘的额外 sidecar（cad：deliver/building.json →
# {"building_text": ...}；ifc：None → 仅 map sidecar）。
SaveSidecars = Callable[[Request, str], Dict[str, str]]

# save 响应附带的版本对齐报告（ifc：bootstrap 原件 vs 本次产物的构件级计数，
# 失败降级 None；cad：None → 响应无 alignment 键）。
Alignment = Callable[[Request, str, str], Optional[Dict[str, int]]]

# locate 查询值 → 构件 design key（ifc：guid → registry 加载 → Pset designKey，
# 缺失返回 None；cad：恒等）。None = 查询值即 key（cad 默认形态）。
LocateKey = Callable[[Request, str, str], Optional[str]]

# buildingChanges 协作件（cad 独有）：需提供 load_building_sidecar /
# diff_building（app.building_diff 模块 duck-type）。
BuildingDiff = Any


@dataclass(frozen=True)
class ScriptsDeps:
    """四个 script-as-source 共享路由模块共用的一份协作件。"""

    runner: Any  # run_script / validate_script_text（app.script_runner shim 模块）
    compute_diff: ComputeDiff
    after_run: Optional[AfterRun] = None
    save_sidecars: Optional[SaveSidecars] = None
    alignment: Optional[Alignment] = None
    locate_key: Optional[LocateKey] = None
    building_diff: Optional[BuildingDiff] = None


@dataclass(frozen=True)
class DiffDeps:
    """POST /diff 语义 diff 路由的协作件（独立于 scripts 装配，两侧单独挂载）。"""

    compute_diff: ComputeDiff
    materialize_version: Any  # (data_dir, model_id, version, settings) -> 产物路径
    # ext 绑定形态 (request, model_id)——经 lambda call-time 转发到 app.route_common
    # 模块属性（cad test_diff 的 model_lock spy patch 在该模块上生效）。
    model_lock: Callable[[Request, str], Any]
    model_upload_path: Callable[[Request, str], str]
