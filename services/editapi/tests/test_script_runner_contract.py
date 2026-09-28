# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_runner 契约门：静态校验在执行前拒绝非契约脚本（flows 契约校验）。

W-0057 T3 收编自两侧 test_script_runner_contract.py（94% 同构）：判定与
原两侧逐条等价，仅产物扩展名经 profile 折算。
"""

from __future__ import annotations

from pathlib import Path

import pytest
from fastapi import HTTPException

from app import script_runner

from conftest import GOOD_SCRIPT, TargetProfile
from script_runner_scripts import (
    NON_LITERAL_PARAMS_SCRIPT,
    NO_BUILD_SCRIPT,
    NO_MAIN_SCRIPT,
    NO_PARAMS_SCRIPT,
)


class TestContractGate:
    """Static validation rejects non-contract scripts before any execution."""

    @pytest.mark.parametrize(
        "script,marker",
        [
            (NO_PARAMS_SCRIPT, "PARAMS"),
            (NO_BUILD_SCRIPT, "build"),
            (NO_MAIN_SCRIPT, "__main__"),
            (NON_LITERAL_PARAMS_SCRIPT, "字面量"),
            ("PARAMS = {", "语法错误"),
        ],
    )
    def test_contract_violations_422(
        self, settings, tmp_path: Path, profile: TargetProfile, script, marker
    ):
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(
                settings, script, str(tmp_path / f"out.{profile.ext}")
            )
        assert exc.value.status_code == 422
        assert marker in str(exc.value.detail)
        assert not (tmp_path / f"out.{profile.ext}").exists()

    def test_validate_script_text_reports_errors(self, settings):
        assert script_runner.validate_script_text(settings, GOOD_SCRIPT) == []
        errors = script_runner.validate_script_text(settings, NO_PARAMS_SCRIPT)
        assert any("PARAMS" in e for e in errors)
