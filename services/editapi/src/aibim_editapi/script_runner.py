# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""共享 script_runner 服务适配层（W-0057 T2 单一源）。

原 services/ifc 与 services/cad 的 app/script_runner.py 合一。沙箱实现仍
单点在 ``aibim_sandbox.runner``；本模块做两件事：

- ``sandbox_config(profile)``：profile 的沙箱静态差异字段 → ``SandboxConfig``
  （inner runner 按需由 flows 模块名生成）；
- ``ScriptRunner``：持有 CONFIG 的每服务适配器，把 Settings 折成 RunSpec
  （drawlib_dir 冒号列表 → 额外 PYTHONPATH/ro-binds；空 = 无注入，ifc 恒空）
  后转发共享 ``run_script`` / ``validate_script_text``。

服务侧 ``app/script_runner.py`` 绑定 CONFIG 后再导出常量与入口——调用点与
测试 seam（``CONFIG``/``MAX_PROCS``/``_spec``/``script_hash`` 等）零改动。

测试 seam 纪律不变：monkeypatch 一律打在 ``aibim_sandbox.backend`` /
``aibim_sandbox.runner`` 模块上（如 ``backend.detect_backend``、
``runner._RUN_GATE``）——本包与两侧 shim 都不别名这些函数。
"""

from __future__ import annotations

from typing import List, Optional

from aibim_sandbox import runner as _runner
from aibim_sandbox.spec import (
    FSIZE_LIMIT_BYTES,
    MAX_PROCS,
    MEM_LIMIT_BYTES,
    OUTPUT_LIMIT_BYTES,
    PRODUCT_LIMIT_BYTES,
    RUN_CONCURRENCY,
    RUN_TIMEOUT_S,
    STDERR_TAIL_BYTES,
    RunSpec,
    SandboxConfig,
)

from .config import Settings
from .profile import ServiceProfile

__all__ = [
    "FSIZE_LIMIT_BYTES",
    "MAX_PROCS",
    "MEM_LIMIT_BYTES",
    "OUTPUT_LIMIT_BYTES",
    "PRODUCT_LIMIT_BYTES",
    "RUN_CONCURRENCY",
    "RUN_TIMEOUT_S",
    "STDERR_TAIL_BYTES",
    "ScriptRunner",
    "reset_state_inner_runner",
    "sandbox_config",
    "script_hash",
]

# inner runner 模板（cad 形态）：先 reset flows 模块状态（确定性 key 计数），
# 再经 runpy 以 __main__ 直跑用户脚本；ifc 不用此层（直跑 script.py）。
_INNER_RUNNER_TEMPLATE = '''\
"""Sandbox inner runner: reset {flows_module} state, then run the user script."""
import runpy
import sys

import {flows_module}

{flows_module}.reset_state()
script_path, out_path = sys.argv[1], sys.argv[2]
sys.argv = [script_path, out_path]
runpy.run_path(script_path, run_name="__main__")
'''


def reset_state_inner_runner(flows_module: str) -> str:
    """由 flows 模块名生成 inner runner 源文本（模块须提供 reset_state）。"""
    return _INNER_RUNNER_TEMPLATE.format(flows_module=flows_module)


def sandbox_config(profile: ServiceProfile) -> SandboxConfig:
    """profile 的沙箱静态差异字段 → SandboxConfig（inner runner 按需生成）。"""
    return SandboxConfig(
        temp_prefix=profile.temp_prefix,
        product_name=profile.product_name,
        product_label=profile.product_label,
        flows_module=profile.flows_module,
        validate_prefix=profile.validate_prefix,
        inner_runner=(
            reset_state_inner_runner(profile.flows_module)
            if profile.reset_state_inner_runner
            else None
        ),
        default_deps=profile.default_deps,
    )


script_hash = _runner.script_hash


class ScriptRunner:
    """每服务一份的 runner 适配：持有 CONFIG，Settings → RunSpec → 共享执行。"""

    def __init__(self, config: SandboxConfig) -> None:
        self.config = config

    def spec(self, settings: Settings) -> RunSpec:
        """settings → RunSpec（drawlib 冒号列表折成额外 PYTHONPATH/ro-binds）。"""
        extra = tuple(d for d in settings.drawlib_dir.split(":") if d)
        return RunSpec(
            config=self.config,
            flows_dir=settings.flows_dir,
            extra_pythonpath=extra,
            extra_ro_binds=extra,
            backend=settings.sandbox_backend,
        )

    def run_script(
        self,
        settings: Settings,
        script_text: str,
        out_path: str,
        *,
        map_out: Optional[str] = None,
        timeout: int = RUN_TIMEOUT_S,
    ) -> None:
        """Validate + execute script_text, publishing the product to out_path.

        语义见 aibim_sandbox.runner.run_script（422 契约/超时/失败/产物与 map
        发布前校验；503 bwrap 不可用 fail-closed；429 并发闸满）。
        """
        _runner.run_script(
            self.spec(settings), script_text, out_path, map_out=map_out, timeout=timeout
        )

    def validate_script_text(self, settings: Settings, script_text: str) -> List[str]:
        """Static contract check (ast, no execution). Empty list = passes."""
        return _runner.validate_script_text(self.spec(settings), script_text)
