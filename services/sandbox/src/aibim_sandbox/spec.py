# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""spec.py — 沙箱运行常量与服务间差异配置（W-0048 T1 单一源）。

常量默认值 = 两侧合一前的公共取值（T0 契约 TestConstants 钉死）。
``SandboxConfig`` 装服务间**静态**差异（产物名/临时前缀/flows 校验器/
inner-runner），``RunSpec`` = SandboxConfig + 每次调用从服务 settings 来的
路径（flows_dir/额外 PYTHONPATH/额外 ro-binds）。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Optional, Tuple

RUN_TIMEOUT_S = 60
MEM_LIMIT_BYTES = 1 << 30  # 1 GiB (RLIMIT_AS, virtual address space)
MAX_PROCS = 256  # RLIMIT_NPROC：防 fork 炸弹
STDERR_TAIL_BYTES = 2048
FSIZE_LIMIT_BYTES = 256 << 20  # RLIMIT_FSIZE 默认（env SCRIPT_MAX_FSIZE_BYTES）
OUTPUT_LIMIT_BYTES = 1 << 20  # stdout+stderr 累计上限（env SCRIPT_MAX_OUTPUT_BYTES）
PRODUCT_LIMIT_BYTES = 256 << 20  # 产物/map 发布上限（env SCRIPT_MAX_PRODUCT_BYTES）
RUN_CONCURRENCY = 3  # run/save 进程级并发闸（env SCRIPT_RUN_CONCURRENCY）


@dataclass(frozen=True)
class SandboxConfig:
    """服务间静态差异配置（import 时确定，运行期不变）。"""

    temp_prefix: str  # 沙箱临时目录前缀（aiifc-run- / aidxf-run-）
    product_name: str  # 沙箱内产物文件名（out.ifc / out.dxf）
    product_label: str  # 缺产物错误消息里的格式标签（IFC / DXF）
    flows_module: str  # flows 契约校验器模块名（script_lib / cad_script_lib）
    validate_prefix: str  # 静态校验临时目录前缀
    # cad 内层 runner（runpy 包装 + reset_state）；None = 直跑 python script.py out
    inner_runner: Optional[str] = None
    # T4 默认依赖集：无 PEP 723 声明的存量脚本注入（uv 内容寻址 env 同源机制）。
    # ifc：("ifcopenshell>=0.8", "numpy")；cad：("ezdxf>=1.3",)。
    default_deps: Tuple[str, ...] = ()
    run_timeout_s: int = RUN_TIMEOUT_S
    mem_limit_bytes: int = MEM_LIMIT_BYTES
    max_procs: int = MAX_PROCS
    stderr_tail_bytes: int = STDERR_TAIL_BYTES


@dataclass(frozen=True)
class RunSpec:
    """单次沙箱执行的完整规格：静态配置 + 每调用路径。"""

    config: SandboxConfig
    flows_dir: str  # flows 校验器/helper 目录（PYTHONPATH 首项 + ro-bind）
    extra_pythonpath: Tuple[str, ...] = field(default=())  # 追加 PYTHONPATH（cad drawlib）
    extra_ro_binds: Tuple[str, ...] = field(default=())  # 追加只读挂载（cad drawlib）
    # 沙箱后端选择（W-0048 T2 显式化，= Settings.sandbox_backend）：
    # auto（默认，bwrap 优先、缺失 fail-closed）/ bwrap / rlimit（仅测试）。
    backend: str = "auto"


DEFAULT_CONFIG = SandboxConfig(
    temp_prefix="aibim-run-",
    product_name="out.bin",
    product_label="PRODUCT",
    flows_module="script_lib",
    validate_prefix="aibim-validate-",
)
