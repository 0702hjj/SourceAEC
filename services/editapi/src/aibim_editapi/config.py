# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""共享 Settings 与 env 配置加载（W-0057 T2 单一源）。

原 services/ifc 与 services/cad 的 app/config.py 合一：字段集合、env 名与
默认值全部来自 ServiceProfile（两侧 app/config.py 声明后经薄 shim 再导出）。

行为零变更基准（T0 契约 + 两侧全量套件）：

- 相对路径（默认 flows_dir）相对**服务根**解析（profile.service_root），
  不是 cwd；
- drawlib_dir 仅当 profile 声明了 env 名/默认路径才可能有值（ifc 恒 ""）；
  缺省推导 = repo 根下按声明顺序拼接**存在**的目录（冒号分隔）；
- data_dir 不在本模块解析（相对值由调用方按需解析，行为同收编前）。
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path

from aibim_sandbox.backend import parse_backend_choice

from .profile import ServiceProfile

DATA_DIR_ENV = "VIEWER_DATA_DIR"
DEFAULT_DATA_DIR = "../data"  # 相对 service_root；两侧一致
SANDBOX_BACKEND_ENV = "SANDBOX_BACKEND"


@dataclass(frozen=True)
class Settings:
    """Runtime settings resolved from environment variables."""

    port: int
    data_dir: str
    flows_dir: str
    drawlib_dir: str = ""  # cad：共享画法层 src 多路径（冒号分隔）；ifc 恒 ""
    max_models: int = 8
    diff_timeout_s: int = 60
    # 沙箱后端（W-0048 T2）：auto（默认，bwrap 优先、缺失 fail-closed 503）/
    # bwrap（显式，不可用即 503）/ rlimit（仅测试可显式选，不隔离网络与沙箱外 FS）。
    sandbox_backend: str = "auto"


def _resolve_path(value: str, anchor: Path) -> str:
    """Resolve a possibly-relative path against the service root (not cwd)."""
    p = Path(value)
    return str(p.resolve()) if p.is_absolute() else str((anchor / p).resolve())


def _default_drawlib_dir(profile: ServiceProfile) -> str:
    """缺省 drawlib：repo 根下按声明顺序拼接**存在**的目录（cad 形态）。

    services/{name} → services → repo root；目录不存在的条目跳过
    （CI 干净克隆无产物也能跑，缺省推导自 skill **源**目录）。
    """
    repo_root = profile.service_root.parent.parent
    candidates = [repo_root / rel for rel in profile.drawlib_repo_rel_paths]
    return ":".join(str(p) for p in candidates if p.is_dir())


def load_settings(profile: ServiceProfile) -> Settings:
    """Build Settings from env（env 名/默认值全部来自 profile）。"""
    env = os.environ
    drawlib_dir = (
        env.get(profile.drawlib_dir_env, "") if profile.drawlib_dir_env else ""
    )
    if not drawlib_dir and profile.drawlib_repo_rel_paths:
        drawlib_dir = _default_drawlib_dir(profile)
    return Settings(
        port=int(env.get(profile.port_env, str(profile.default_port))),
        data_dir=env.get(DATA_DIR_ENV, DEFAULT_DATA_DIR),
        flows_dir=_resolve_path(
            env.get(profile.flows_dir_env, profile.default_flows_dir),
            profile.service_root,
        ),
        drawlib_dir=drawlib_dir,
        max_models=int(
            env.get(profile.max_models_env, str(profile.default_max_models))
        ),
        diff_timeout_s=int(
            env.get(profile.diff_timeout_env, str(profile.default_diff_timeout_s))
        ),
        sandbox_backend=parse_backend_choice(env.get(SANDBOX_BACKEND_ENV, "auto")),
    )
