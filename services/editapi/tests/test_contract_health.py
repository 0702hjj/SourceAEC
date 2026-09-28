# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Health endpoint contract（W-0060 T1 收编两侧 0-diff 镜像 test_health.py）。

两侧原文件逐字节一致（各 1 用例）；共享后同用例在两 venv 各执行一次，
执行数守恒，断言原样。
"""

from __future__ import annotations


def test_health(client) -> None:
    resp = client.get("/health")
    assert resp.status_code == 200
    assert resp.json() == {"status": "ok"}
