# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""CAD run 链路服务特有钩子（W-0057 T1 薄适配；共享路由在
aibim_editapi.routes_script_run）。

CAD 差异（全部经 ScriptsDeps 注入，共享代码零分叉）：

- ``AFTER_RUN``：run 成功后原子发布 render.json（实现在 app/render_publish.py，
  W-0059 自 render.py 拆出；tmp+os.replace、失败删旧防错位的纪律不变）。
- ``SAVE_SIDECARS``：交付产物 deliver/building.json 随大版本 lockstep 快照
  为 scripts/v{n}.building.json（缺失则不落 sidecar，同 map 纪律）。
- ``COMPUTE_DIFF`` 经 lambda 转发——tests monkeypatch 的是 app.dxf_diffing
  模块属性（call-time 查找，patch seam 不变）。save 响应无 alignment 键
  （chunk A 决策不变）。
"""

import os
from typing import Dict

from fastapi import Request

from . import dxf_diffing, render_publish

__all__ = ["AFTER_RUN", "COMPUTE_DIFF", "SAVE_SIDECARS"]

COMPUTE_DIFF = lambda old, new: dxf_diffing.compute_diff(old, new)  # noqa: E731

AFTER_RUN = render_publish.publish_render_json


def SAVE_SIDECARS(request: Request, model_id: str) -> Dict[str, str]:
    """大版本 sidecar 收集：building.json 在 → building_text；缺失 → 空 dict。"""
    building_path = os.path.join(
        request.app.state.settings.data_dir, "models", model_id, "deliver", "building.json"
    )
    if not os.path.isfile(building_path):
        return {}
    with open(building_path, "r", encoding="utf-8") as fh:
        return {"building_text": fh.read()}
