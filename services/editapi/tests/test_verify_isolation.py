# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""校验隔离契约测试（机器强制，W-0024；W-0060 T3 收编两侧镜像为单份）。

AGENTS.md「校验与业务隔离」硬规则：业务规则校验必须住在 ``verify*``/``validate*``
函数里；handler 内禁止内联 ``raise HTTPException``。本测试用 ast 解析目标服务
``app/routes_*.py`` 与共享领域包 ``aibim_editapi`` 的 src（W-0057 T2 起领域
实现进共享包，扫描范围随之扩展），断言 ``raise HTTPException`` 只出现在：

- 名为 ``verify*``/``validate*`` 的函数体内，或
- 模块 ``route_common.py`` 内（单点 helper）。

判定粒度：raise 所在的「最小函数」。若该函数名不是 verify*/validate* 且模块不是
route_common，即违规。别名 import（``from fastapi import HTTPException as H``）的
``raise H(...)`` 按同一判定（W-0024 逃逸补丁，见自证）。

机器强制的目标是「新代码不得违规」：存量违规以显式 ``ALLOWLIST`` 登记（保证绿），
新 handler 内联 raise 不在白名单 → 变红。存量违规的收拢无专项 deadline（W-0024
「存量逐步推开」：不做一次性大改，触碰到的 handler 顺手收拢）。

W-0060 T3：原两侧各持一份逐行镜像的检查器（cad 版自述「镜像 services/ifc
同款」），本文件以 ifc 版为准合一（``_enter``/``_exit`` 结构），扫描目标由
TARGET 推导，白名单按 target 分字典（差异单点可读，测试数据留在测试文件，
不进 TargetProfile）。
"""

from __future__ import annotations

import ast
import re
from pathlib import Path

from target_profiles import EDITAPI_ROOT, SERVICES_ROOT, TARGET

ROUTES_DIR = SERVICES_ROOT / TARGET / "app"

# 共享领域包（W-0057 T2）：app/ 只剩 shim 后，隔离规则的实质实现住在
# aibim_editapi src——同规则扫描（editapi 无存量违规，不进 ALLOWLIST）。
EDITAPI_SRC_DIR = EDITAPI_ROOT / "src" / "aibim_editapi"

VERIFY_RE = re.compile(r"^(verify|validate)")

# 存量违规白名单：{target: {module: {违规所在最小函数名}}}。
# 归属说明（报告见 .superpowers/sdd/2026-08-10-script-closure/task-1-report.md）：
# - 全部为 b78242a（AGENTS 硬规则入约）之前遗留，收拢无专项 deadline，随触碰收拢；
# - routes_scripts.py 的内联 raise 已于 W-0048 T3 随四域拆分全部下沉 verify*（对齐
#   services/cad 形态），该模块（现装配层）白名单项随之清零；
# - routes_diff.py 的 _version_or_404/_run_diff_with_timeout 已于 W-0057 T1 随
#   diff 路由合一进 aibim_editapi.routes_diff 收拢为 verify* 形态（共享包无白名单），
#   白名单项随之清零；
# - cad 侧从空集起步（新服务无存量违规），任何 handler 内联 raise 直接变红。
# 新增 handler 内联 raise HTTPException（非 verify*/validate* 函数）→ 本测试变红。
ALLOWLISTS: dict[str, dict[str, set[str]]] = {
    "ifc": {
        "routes_edits.py": {"_retired"},
        "routes_user_edits.py": {"post_diff_upload", "post_user_edits"},
    },
    "cad": {},
}

ALLOWLIST = ALLOWLISTS[TARGET]


def violations_in_module(tree: ast.AST, module_name: str) -> list[tuple[str, int]]:
    """返回违规点：(最小函数名, 行号)。

    判定：``raise HTTPException`` 所在的最小函数名既非 verify*/validate*、
    模块又非 route_common.py，即违规。verify*/validate* 函数内 raise 合法；
    route_common.py 整体豁免（单点 helper 的合法翻译点）。

    W-0024 逃逸补丁：别名 import（``from fastapi import HTTPException as H``）的
    ``raise H(...)`` 按同一判定（别名名纳入异常名集合）。作用域内重绑定
    （函数内局部 ``H = ...`` 遮蔽）不在契约测试范围。
    """
    if module_name == "route_common.py":
        return []
    found: list[tuple[str, int]] = []
    # 收集模块级别名：``from fastapi import HTTPException as H`` → H 也算。
    exception_names = {"HTTPException"}
    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom) and node.module == "fastapi":
            for alias in node.names:
                if alias.name == "HTTPException":
                    exception_names.add(alias.asname or "HTTPException")

    class _V(ast.NodeVisitor):
        def __init__(self) -> None:
            self.stack: list[str] = []

        def _enter(self, name: str) -> None:
            self.stack.append(name)

        def _exit(self) -> None:
            self.stack.pop()

        def visit_FunctionDef(self, node: ast.FunctionDef) -> None:
            self._enter(node.name)
            self.generic_visit(node)
            self._exit()

        def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef) -> None:
            self._enter(node.name)
            self.generic_visit(node)
            self._exit()

        def visit_Raise(self, node: ast.Raise) -> None:
            if (
                isinstance(node.exc, ast.Call)
                and isinstance(node.exc.func, ast.Name)
                and node.exc.func.id in exception_names
            ):
                func = self.stack[-1] if self.stack else "<module>"
                if not VERIFY_RE.match(func):
                    found.append((func, node.lineno))
            self.generic_visit(node)

    _V().visit(tree)
    return found


def route_modules() -> list[Path]:
    return sorted(ROUTES_DIR.glob("routes_*.py")) + sorted(EDITAPI_SRC_DIR.glob("*.py"))


def test_current_route_files_raise_only_in_verify_or_allowlisted():
    """当前代码库：所有违规点必须显式登记在 ALLOWLIST（新违规 → 红）。"""
    unlisted = []
    for path in route_modules():
        for func, line in violations_in_module(
            ast.parse(path.read_text(encoding="utf-8"), filename=str(path)), path.name
        ):
            if func not in ALLOWLIST.get(path.name, set()):
                unlisted.append(f"{path.name}:{line} {func}")
    assert not unlisted, "以下 handler/helper 内联 raise HTTPException 未登记白名单（新违规）:\n" + "\n".join(unlisted)


def test_allowlist_entries_are_real_violations():
    """白名单只登记真实存量违规（无 stale 项）——收拢后必须同步删除。"""
    real = {
        path.name: {
            func
            for func, _ in violations_in_module(
                ast.parse(path.read_text(encoding="utf-8"), filename=str(path)), path.name
            )
        }
        for path in route_modules()
    }
    stale = []
    for module, funcs in ALLOWLIST.items():
        if module not in real:
            stale.append(f"{module}（模块已不存在）")
        for func in funcs:
            if func not in real.get(module, set()):
                stale.append(f"{module}:{func}（已无违规）")
    assert not stale, "ALLOWLIST 存在失效项，收拢后请同步删除:\n" + "\n".join(stale)


# --- 自证：检查逻辑有区分力（对违规样例断言会变红）---

VIOLATING_SAMPLE = """\
from fastapi import APIRouter, HTTPException

router = APIRouter()


@router.get("/models/{id}/script")
def get_script(id: str) -> str:
    if not id:
        raise HTTPException(status_code=404, detail="no script for model")
    return id
"""

VERIFY_SAMPLE = """\
from fastapi import HTTPException


def verify_script_body(body):
    if body is None:
        raise HTTPException(status_code=422, detail="body required")
"""

COMMON_SAMPLE = """\
from fastapi import HTTPException


def model_upload_path(request, model_id):
    raise HTTPException(status_code=404, detail="model not found")
"""


def test_checker_detects_handler_inline_raise():
    """自证：handler 内联 raise HTTPException 必须被检出（变红）。"""
    tree = ast.parse(VIOLATING_SAMPLE)
    viol = violations_in_module(tree, "routes_x.py")
    assert ("get_script", 9) in viol, f"检查逻辑未检出违规 handler: {viol}"


def test_checker_accepts_verify_function_raise():
    """自证：verify*/validate* 函数内 raise 合法，不得误报。"""
    tree = ast.parse(VERIFY_SAMPLE)
    assert violations_in_module(tree, "routes_x.py") == []


def test_checker_accepts_route_common_module():
    """自证：route_common.py 模块整体豁免，不得误报。"""
    tree = ast.parse(COMMON_SAMPLE)
    assert violations_in_module(tree, "route_common.py") == []
    # 同一代码若放在 routes_* 里则必须检出（模块豁免是真实豁免，不是检查逻辑失效）
    assert ("model_upload_path", 5) in violations_in_module(tree, "routes_x.py")


ALIAS_VIOLATING_SAMPLE = """\
from fastapi import APIRouter, HTTPException as H

router = APIRouter()


@router.get("/models/{id}/script")
def get_script_alias(id: str) -> str:
    if not id:
        raise H(status_code=404, detail="no script for model")
    return id
"""

ALIAS_VERIFY_SAMPLE = """\
from fastapi import HTTPException as H


def verify_script_body_alias(body):
    if body is None:
        raise H(status_code=422, detail="body required")
"""


def test_checker_detects_alias_import_raise():
    """自证：别名 import（``HTTPException as H``）的 ``raise H(...)`` 必须被检出。

    W-0024 逃逸补丁：检查若只匹配字面 ``HTTPException`` 标识符，别名 import 可
    绕过（raise 体完全相同，仅名不同）。检出别名即按同一判定。
    """
    tree = ast.parse(ALIAS_VIOLATING_SAMPLE)
    viol = violations_in_module(tree, "routes_x.py")
    assert ("get_script_alias", 9) in viol, f"检查逻辑未检出别名 raise: {viol}"


def test_checker_accepts_verify_function_alias_raise():
    """自证：别名 raise 落在 verify* 函数内仍合法，不得误报。"""
    tree = ast.parse(ALIAS_VERIFY_SAMPLE)
    assert violations_in_module(tree, "routes_x.py") == []
