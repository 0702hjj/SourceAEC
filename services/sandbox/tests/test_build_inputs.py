# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
import base64
import pytest
from aibim_sandbox import build_inputs


def test_input_snapshot_is_private_and_restored(tmp_path):
    with build_inputs.use({"plans/example/plan.json": base64.b64encode(b"{}").decode()}):
        target = build_inputs.project(str(tmp_path))
        assert (tmp_path / ".build-inputs/plans/example/plan.json").read_bytes() == b"{}"
        assert str(target).startswith(str(tmp_path))
        assert build_inputs.active()
    assert not build_inputs.active()


def test_rejects_input_path_traversal(tmp_path):
    with build_inputs.use({"../escape": "eA=="}):
        with pytest.raises(ValueError, match="invalid build input path"):
            build_inputs.project(str(tmp_path))
    assert not (tmp_path.parent / "escape").exists()
