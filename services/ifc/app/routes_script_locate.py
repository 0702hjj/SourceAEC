# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""routes_script_locate 薄适配：共享路由 aibim_editapi.routes_script_locate
（W-0057 T1 单一源）+ IFC 的 guid → designKey 解析器。

IFC 差异（经 ScriptsDeps.locate_key 注入）：locate 查询参数是构件 guid，
先经 registry 加载 IFC、取 Pset_AIIFC.designKey；构件不存在 → 404，
无 designKey → 共享路由回 ``{"found": False}``。cad 为 key 直查（缺省恒等）。
"""

from typing import Any, Optional

import ifcopenshell.util.element
from fastapi import HTTPException, Request

from aibim_sandbox.route_common import model_upload_path

__all__ = ["LOCATE_KEY", "verify_element_by_guid"]


def verify_element_by_guid(model: Any, guid: str) -> Any:
    """guid 必须命中已加载 IFC 模型中的元素（404 的唯一翻译点）。"""
    try:
        return model.by_guid(guid)
    except RuntimeError:
        raise HTTPException(status_code=404, detail=f"element not found: {guid}")


def LOCATE_KEY(request: Request, model_id: str, guid: str) -> Optional[str]:
    """guid → designKey：registry 加载 IFC → 元素 → Pset_AIIFC.designKey。"""
    ifc_path = model_upload_path(request, model_id, "ifc")
    model = request.app.state.registry.load(ifc_path)
    element = verify_element_by_guid(model, guid)
    psets = ifcopenshell.util.element.get_psets(element)
    return (psets.get("Pset_AIIFC") or {}).get("designKey")
