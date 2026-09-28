# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""W-0057 T0 共享 REST 编辑面契约套件：夹具机制。

同一套契约测试代码在两个互斥 venv（ifc=ifcopenshell / cad=ezdxf）下各跑
一遍，进程内只装一个目标服务。目标解析与全部服务差异实现自 W-0060 T2 起
拆至 ``target_profiles.py``（行数门禁驱动，职责单一）；本文件只留夹具与
script_runner 样例物化，并再导出测试文件惯用的导入面（``from conftest
import ...`` 不变）。

W-0057 T3 收编 script_runner 领域套件后，本文件物化
``script_runner_scripts`` 的 target 差异样例（GOOD/越界写/输出洪泛等）并
提供 ``settings`` 夹具（镜像两侧服务 conftest）。
"""

from __future__ import annotations

from pathlib import Path

import pytest

from script_runner_scripts import (
    PRODUCT_PAYLOADS,
    big_map_script,
    escape_write_script,
    good_script,
    good_write_stmt,
    moderate_stdout_script,
    no_output_script,
    stdout_flood_script,
)
from target_profiles import (  # noqa: F401 —— 再导出保持测试导入面不变
    CONTRACT_VIOLATION_SCRIPT,
    FAILING_SCRIPT,
    MODEL_ID,
    UNKNOWN_MODEL_ID,
    PROFILE,
    SERVICE_ROOT,
    TARGET,
    TargetProfile,
    _traced_variant,
    staging_script,
)

# ---------------------------------------------------------------------------
# script_runner 样例物化（W-0057 T3：target 差异在此单点折算；同构常量
# 直接从 script_runner_scripts 导入）
# ---------------------------------------------------------------------------

_PRODUCT_PAYLOAD = PRODUCT_PAYLOADS[TARGET]

GOOD_WRITE_STMT = good_write_stmt(_PRODUCT_PAYLOAD)
GOOD_SCRIPT = good_script(_PRODUCT_PAYLOAD)
NO_OUTPUT_SCRIPT = no_output_script(PROFILE.ext)
ESCAPE_WRITE_SCRIPT = escape_write_script(_PRODUCT_PAYLOAD)
STDOUT_FLOOD_SCRIPT = stdout_flood_script(_PRODUCT_PAYLOAD)
MODERATE_STDOUT_SCRIPT = moderate_stdout_script(_PRODUCT_PAYLOAD)
BIG_MAP_SCRIPT = big_map_script(_PRODUCT_PAYLOAD)


# ---------------------------------------------------------------------------
# 沙箱后端夹具（镜像两侧 conftest：无 bwrap 显式选 rlimit；探针缓存隔离）
# ---------------------------------------------------------------------------

@pytest.fixture(autouse=True)
def _rlimit_backend_when_no_bwrap(monkeypatch: pytest.MonkeyPatch):
    from aibim_sandbox import backend as sandbox_backend

    if sandbox_backend.detect_backend() != "bwrap":
        monkeypatch.setenv("SANDBOX_BACKEND", "rlimit")


@pytest.fixture(autouse=True)
def _probe_cache_isolated():
    yield
    from aibim_sandbox import backend as sandbox_backend

    sandbox_backend._PROBE_OK = None


# ---------------------------------------------------------------------------
# 对外夹具（等价于两侧 conftest 的 client/settings/data_dir + profile 差异面）
# ---------------------------------------------------------------------------

@pytest.fixture()
def profile() -> TargetProfile:
    return PROFILE


@pytest.fixture()
def target() -> str:
    return TARGET


@pytest.fixture()
def model_id() -> str:
    return MODEL_ID


@pytest.fixture()
def settings():
    """目标服务的 Settings（镜像两侧服务 conftest 的 settings 夹具）。"""
    from app.config import load_settings

    return load_settings()


@pytest.fixture()
def data_dir(tmp_path: Path) -> Path:
    return PROFILE.make_data_dir(tmp_path)


@pytest.fixture()
def product_path(tmp_path: Path) -> Path:
    """产物种子文件（ifc：fixture 字节拷贝 / cad：真 DXF）——镜像两侧服务
    conftest 的 ifc_path / dxf_path 夹具（W-0060 T1 单点化）。"""
    return PROFILE.seed_product(tmp_path)


@pytest.fixture()
def client(data_dir: Path, monkeypatch: pytest.MonkeyPatch):
    from fastapi.testclient import TestClient

    from app.main import create_app

    monkeypatch.setenv("VIEWER_DATA_DIR", str(data_dir))
    if TARGET == "cad":
        monkeypatch.setenv("AIDXF_FLOWS_DIR", str(SERVICE_ROOT / "flows"))
    return TestClient(create_app())
