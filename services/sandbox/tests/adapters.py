# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""adapters.py — 双服务配置适配器（W-0048 T1 起：同一共享实现，两套服务配置）。

T0 时本层装载两套**实现**（ifc/cad 各自 script_runner）钉公共行为；T1 合一
后两侧实现都是 ``aibim_sandbox``，参数化改为「双服务配置」——经合成包
（synthetic package）装载各服务的薄适配 ``app/script_runner``（拿到
``CONFIG``/``_spec``/``run_script``），同一组契约跑两套服务配置，验证共享
runner 正确兑现每个服务的参数化差异（产物名/临时前缀/flows 校验器/
inner-runner/额外挂载）。

适配器声明服务间**配置差异**（不断言一致）：

- ``temp_prefix``：沙箱临时目录前缀（``aiifc-run-`` / ``aidxf-run-``）
- ``product_name``：沙箱内产物文件名（``out.ifc`` / ``out.dxf``）
- ``flows_module``：flows 校验器/helper 模块名（``script_lib`` / ``cad_script_lib``）
- ``default_deps``：T4 默认依赖集（无 PEP 723 声明的存量脚本注入；
  ifc ``ifcopenshell>=0.8`` + ``numpy``，cad ``ezdxf>=1.3``）

``facade``（SimpleNamespace）把共享模块的函数按契约测试的调用形状组装
（_limits/_sandbox_env/_sandbox_cmd 等签名在 T1 参数化后变了，此处包回
旧形状，契约用例正文不随接缝迁移改动）。T2 起后端选择走 Settings
（``with_backend`` 做 settings 副本），monkeypatch detect_backend 的
接缝退役；剩余 monkeypatch 点只有 ``adapter.impl`` 的并发闸。
"""

from __future__ import annotations

import dataclasses
import importlib
import sys
import types
from dataclasses import dataclass
from pathlib import Path
from types import ModuleType, SimpleNamespace
from typing import Callable

from aibim_sandbox import backend, deps, runner, spec as _spec_mod

REPO_ROOT = Path(__file__).resolve().parents[3]

_CONSTANTS = (
    "RUN_TIMEOUT_S",
    "MEM_LIMIT_BYTES",
    "MAX_PROCS",
    "STDERR_TAIL_BYTES",
    "FSIZE_LIMIT_BYTES",
    "OUTPUT_LIMIT_BYTES",
    "PRODUCT_LIMIT_BYTES",
    "RUN_CONCURRENCY",
)


@dataclass(frozen=True)
class RunnerAdapter:
    """一套服务配置 + 共享实现 facade。"""

    name: str
    runner: SimpleNamespace  # 契约测试调用面（run_script/units 函数/常量）
    impl: ModuleType  # 共享实现模块（两适配器同一对象——单实现断言用）
    make_settings: Callable[[], object]
    temp_prefix: str
    product_name: str
    flows_module: str
    default_deps: tuple
    backend_module: ModuleType

    def with_backend(self, settings, backend_name: str):
        """settings 副本 + 显式后端选择（T2：选择走 Settings，不 monkeypatch）。

        backend_name ∈ {auto, bwrap, rlimit}；rlimit 仅测试场景。
        """
        return dataclasses.replace(settings, sandbox_backend=backend_name)

    def reset_probe_cache(self) -> None:
        """清 bwrap 探针缓存（PATH 遮蔽模拟无 bwrap 环境的前后置）。"""
        self.backend_module._PROBE_OK = None


def _load_app_package(alias: str, app_dir: Path) -> None:
    """把 app 目录注册为名为 alias 的合成包（相对导入由此可用）。"""
    if alias in sys.modules:
        return
    pkg = types.ModuleType(alias)
    pkg.__path__ = [str(app_dir)]
    sys.modules[alias] = pkg


def _facade(service_runner: ModuleType) -> SimpleNamespace:
    """共享实现 + 服务薄适配 → 契约测试调用面（T0 形状）。"""
    ns = SimpleNamespace()
    ns.run_script = service_runner.run_script
    ns.validate_script_text = service_runner.validate_script_text
    ns.script_hash = runner.script_hash
    ns.detect_backend = backend.detect_backend
    ns.verify_sandbox_backend = lambda settings: backend.verify_sandbox_backend(
        service_runner._spec(settings)
    )
    ns.sandbox_probe_cmd = backend._probe_cmd
    ns._int_env = runner._int_env
    ns._tail = lambda data, limit=None: runner._tail(
        data, limit if limit is not None else _spec_mod.STDERR_TAIL_BYTES
    )
    ns._OutputGuard = runner._OutputGuard
    ns._nproc_budget = lambda: backend._nproc_budget(_spec_mod.MAX_PROCS)
    ns._limits = lambda budget, fsize: backend._limits(
        service_runner.CONFIG, budget, fsize
    )
    ns._sandbox_env = lambda settings, workdir: backend.sandbox_env(
        service_runner._spec(settings), workdir
    )
    ns._sandbox_cmd = lambda settings, workdir, extra_binds=(): backend.sandbox_cmd(
        service_runner._spec(settings), workdir, extra_binds=extra_binds
    )
    # T4：PEP 723 依赖声明 + uv 内容寻址环境
    ns.DepsEnv = deps.DepsEnv
    ns.parse_pep723_deps = deps.parse_pep723_deps
    ns.env_key = deps.env_key
    ns.cache_root = deps.cache_root
    ns.deps_for_script = lambda settings, text: deps.deps_for_script(
        service_runner._spec(settings), text
    )
    ns.ensure_env = lambda settings, text: deps.ensure_env(
        service_runner._spec(settings), text
    )
    for const in _CONSTANTS:
        setattr(ns, const, getattr(_spec_mod, const))
    return ns


def _build_adapter(
    name: str,
    alias: str,
    temp_prefix: str,
    product_name: str,
    flows_module: str,
    default_deps: tuple,
) -> RunnerAdapter:
    app_dir = REPO_ROOT / "services" / name / "app"
    _load_app_package(alias, app_dir)
    config = importlib.import_module(f"{alias}.config")
    service_runner = importlib.import_module(f"{alias}.script_runner")
    return RunnerAdapter(
        name=name,
        runner=_facade(service_runner),
        impl=runner,
        make_settings=config.load_settings,
        temp_prefix=temp_prefix,
        product_name=product_name,
        flows_module=flows_module,
        default_deps=default_deps,
        backend_module=backend,
    )


_CACHE: dict[str, RunnerAdapter] = {}

_SPECS = {
    "ifc": ("svc_ifc_app", "aiifc-run-", "out.ifc", "script_lib",
            ("ifcopenshell>=0.8", "numpy")),
    "cad": ("svc_cad_app", "aidxf-run-", "out.dxf", "cad_script_lib",
            ("ezdxf>=1.3",)),
}


def get_adapter(name: str) -> RunnerAdapter:
    """按名取适配器（懒装载，进程内缓存）。"""
    if name not in _CACHE:
        alias, prefix, product, flows, default_deps = _SPECS[name]
        _CACHE[name] = _build_adapter(name, alias, prefix, product, flows,
                                      default_deps)
    return _CACHE[name]


ADAPTER_NAMES = tuple(_SPECS)


def require_bwrap(adapter: RunnerAdapter) -> None:
    """仅 bwrap 后端生效的隔离契约（越界写/断网/挂载收窄）的运行时守卫。

    CI 无 bwrap：这些用例 skip，真跑契约由 rlimit 路径覆盖其余部分。
    """
    import pytest

    if adapter.runner.detect_backend() != "bwrap":
        pytest.skip("该隔离契约仅 bwrap 后端生效（rlimit 降级不隔离 FS/网络）")
