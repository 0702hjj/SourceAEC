# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_edit 标量重写纯函数套件（W-0060 T2 收编两侧镜像 TestRewriteCallArgument）。

``rewrite_call_argument`` 实现单点在 aibim_sandbox.script_edit（W-0048 T1），
本套件经 ``from app import script_edit`` 走目标服务 shim，连带钉住绑定；
12 用例与两侧原文件逐条一致（纯函数，零服务差异，无需 profile）。
"""

from __future__ import annotations

import pytest

from app import script_edit


class TestRewriteCallArgument:
    def test_rewrite_string_argument(self):
        script = 'w = create_entity(model, "IfcWall", key="s1:wall:1", name="old")\n'
        out = script_edit.rewrite_call_argument(script, 1, "name", "new")
        assert '"new"' in out and '"old"' not in out
        assert 'key="s1:wall:1"' in out

    def test_rewrite_int_argument(self):
        script = 'w = create_entity(model, "IfcWall", key="k", count=1)\n'
        out = script_edit.rewrite_call_argument(script, 1, "count", 42)
        assert "count=42" in out

    def test_rewrite_float_argument(self):
        script = 'w = create_entity(model, "IfcWall", key="k", height=3.0)\n'
        out = script_edit.rewrite_call_argument(script, 1, "height", 2.5)
        assert "height=2.5" in out

    def test_rewrite_bool_argument_not_int(self):
        """bool 是 int 子类：True 必须写成 True 而不是 1。"""
        script = 'w = create_entity(model, "IfcWall", key="k", flag=False)\n'
        out = script_edit.rewrite_call_argument(script, 1, "flag", True)
        assert "flag=True" in out
        assert "flag=1" not in out

    def test_rewrite_appends_missing_argument(self):
        script = 'w = create_entity(model, "IfcWall", key="k")\n'
        out = script_edit.rewrite_call_argument(script, 1, "name", "W9")
        assert 'name="W9"' in out

    def test_rewrite_only_touches_target_line(self):
        script = (
            'a = create_entity(model, "IfcWall", key="k1", name="one")\n'
            'b = create_entity(model, "IfcWall", key="k2", name="two")\n'
        )
        out = script_edit.rewrite_call_argument(script, 2, "name", "TWO")
        assert 'name="one"' in out
        assert 'name="TWO"' in out

    def test_rewrite_preserves_comments_and_blank_lines(self):
        """libcst 无损：注释、空行、缩进原样保留。"""
        script = (
            "# header comment\n"
            "\n"
            "def build(params, out_path):\n"
            "    # inner comment\n"
            "    w = create_entity(model, \"IfcWall\", key=\"k\", name=\"old\")  # tail\n"
            "\n"
            "    return w\n"
        )
        out = script_edit.rewrite_call_argument(script, 5, "name", "new")
        assert out == script.replace('name="old"', 'name="new"')

    def test_rewrite_no_call_at_line_raises(self):
        with pytest.raises(ValueError):
            script_edit.rewrite_call_argument("x = 1\n", 1, "name", "v")

    def test_rewrite_syntax_error_raises_valueerror(self):
        with pytest.raises(ValueError):
            script_edit.rewrite_call_argument("def broken(:\n", 1, "name", "v")

    @pytest.mark.parametrize("bad", [{"a": 1}, [1, 2], (1,), None])
    def test_rewrite_rejects_non_scalar_value(self, bad):
        """容器/None 注入 → ValueError（只允许 str/int/float/bool 字面量）。"""
        with pytest.raises(ValueError):
            script_edit.rewrite_call_argument(
                'w = f(key="k", name="x")\n', 1, "name", bad
            )

    @pytest.mark.parametrize("bad_arg", ["na me", '**{"k": 1}, x', "", "1abc", "name;x"])
    def test_rewrite_rejects_non_identifier_argument(self, bad_arg):
        """非法参数名会让 libcst 抛 CSTValidationError(非 ValueError) → 提前 422。"""
        with pytest.raises(ValueError):
            script_edit.rewrite_call_argument(
                'w = f(key="k", name="x")\n', 1, bad_arg, "v"
            )

    @pytest.mark.parametrize("bad_float", [float("nan"), float("inf"), float("-inf")])
    def test_rewrite_rejects_non_finite_float(self, bad_float):
        """nan/inf 的 repr 不是合法 Python 字面量 → ValueError。"""
        with pytest.raises(ValueError):
            script_edit.rewrite_call_argument(
                'w = f(key="k", height=3.0)\n', 1, "height", bad_float
            )
