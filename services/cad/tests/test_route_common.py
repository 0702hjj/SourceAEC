# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""route_common 锁源测试：共享 ModelLocks 的同 key 同锁与 LRU 逐出上限。

W-0048 T1：锁表实现上移 aibim_sandbox.route_common.ModelLocks（app 侧
route_common 薄适配经 request.app.state.model_locks 取用，main.py 安装）。
"""

from __future__ import annotations

import threading

from aibim_sandbox.route_common import LOCKS_MAX, ModelLocks


def test_model_locks_returns_same_lock_per_key() -> None:
    locks = ModelLocks()
    assert locks.lock("m_a") is locks.lock("m_a")
    assert isinstance(locks.lock("m_b"), type(threading.RLock()))


def test_model_locks_lru_eviction_beyond_cap() -> None:
    """锁表上限：超出丢最旧；被访问的条目 move_to_end 不被逐出。"""
    cap = 8
    locks = ModelLocks(max_locks=cap)
    for i in range(cap + 1):
        locks.lock(f"m_{i:016x}")
    assert len(locks._locks) == cap
    assert f"m_{0:016x}" not in locks._locks  # 最旧被逐出
    assert f"m_{cap:016x}" in locks._locks

    # 访问最旧的存活条目 → 移到最新；再插入一个 → 逐出次旧而非它
    kept = f"m_{1:016x}"
    locks.lock(kept)
    locks.lock(f"m_{cap + 1:016x}")
    assert kept in locks._locks
    assert f"m_{2:016x}" not in locks._locks
    assert len(locks._locks) == cap
    assert ModelLocks()._max == LOCKS_MAX
