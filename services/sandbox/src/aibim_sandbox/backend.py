# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""backend.py — 沙箱后端探测 + 资源限制 + 运行环境/命令构造（W-0048 T1 合一，
T2 后端选择显式化 + 单后端收敛）。

合并原 services/ifc app/script_runner.py 与 services/cad app/sandbox_exec.py
的同构部分；常量全部参数化（经 SandboxConfig/RunSpec 传入），消灭 cad 的
``_bind_constants`` 可变全局 hack。

- **bwrap backend**（生产唯一路径）：按需只读挂载（/usr·/lib·解释器前缀·
  flows_dir·extra_ro_binds）+ tmpfs /tmp + 可写 sandbox cwd +
  ``--unshare-net``——**不挂 /data、不挂 /etc**（整根只读挂载会让脚本读到
  其他租户的模型，W-0047 跨租户读洞）。
- **fail-closed（T2）**：后端选择由 ``RunSpec.backend``（= 服务 Settings
  的 ``sandbox_backend``，env ``SANDBOX_BACKEND``）显式给出——``auto``
  （默认）探测不到 bwrap、或显式 ``bwrap`` 而 bwrap 不可用时，run/save
  一律 503；旧 ``ALLOW_RLIMIT_FALLBACK`` env 放行语义已删除（生产 env
  不再影响判定）。
- **rlimit 测试后端**（``sandbox_backend=rlimit`` 显式选择，仅测试场景）：
  same rlimits + isolated cwd/TMPDIR；FS 越界写与网络**不拦截**——不是
  生产降级路径。
- 探针保真（W-0048 T0 裁决点②）：探测命令与真跑同一挂载形态（含
  ``--tmpfs /tmp`` + ``--proc /proc``）——旧探针无 tmpfs，repo 落在 /tmp
  下时探针过、真跑 execvp 失败（venv 被 tmpfs 遮蔽）。
"""

from __future__ import annotations

import logging
import os
import shutil
import subprocess
import sys
from typing import Dict, List, Optional, Tuple

from fastapi import HTTPException

from .spec import RunSpec, SandboxConfig

logger = logging.getLogger(__name__)

# 后端选择值域（Settings.sandbox_backend / RunSpec.backend，env SANDBOX_BACKEND）。
BACKEND_AUTO = "auto"  # 默认：bwrap 可用则 bwrap，否则 fail-closed（503）
BACKEND_BWRAP = "bwrap"  # 显式 bwrap：不可用即 503，不静默降级
BACKEND_RLIMIT = "rlimit"  # 显式 rlimit：仅测试场景（不隔离网络与沙箱外 FS）
BACKEND_CHOICES = (BACKEND_AUTO, BACKEND_BWRAP, BACKEND_RLIMIT)

_PROBE_OK: Optional[bool] = None


def _limits(cfg: SandboxConfig, nproc: int, fsize: int) -> None:
    """preexec_fn: apply resource limits (inherited by bwrap and its child)."""
    import resource

    resource.setrlimit(resource.RLIMIT_CPU, (cfg.run_timeout_s + 30, cfg.run_timeout_s + 60))
    resource.setrlimit(resource.RLIMIT_AS, (cfg.mem_limit_bytes, cfg.mem_limit_bytes))
    resource.setrlimit(resource.RLIMIT_NPROC, (nproc, nproc))
    # 单文件写上限：防脚本写满 /data 卷（产物发布前的 product 校验之外的
    # 内核层硬闸，bwrap/rlimit 两后端都生效）。
    resource.setrlimit(resource.RLIMIT_FSIZE, (fsize, fsize))


def _nproc_budget(max_procs: int) -> int:
    """RLIMIT_NPROC 目标值：当前 uid 的 task 数 + max_procs 余量。

    RLIMIT_NPROC 按 uid 的全部 task（含既有线程）计数，固定上限会让高线程
    环境下的沙箱连 fork/userns 都建不了（EAGAIN）；「现有 + 余量」把脚本
    能新增的进程数约束在 max_procs 以内，与环境无关。
    """
    uid = os.getuid()
    current = 0
    for entry in os.listdir("/proc"):
        if not entry.isdigit():
            continue
        try:
            if os.stat(f"/proc/{entry}").st_uid != uid:
                continue
            current += len(os.listdir(f"/proc/{entry}/task"))
        except OSError:
            continue
    return current + max_procs


def _runtime_ro_binds() -> List[str]:
    """运行时按需只读挂载：系统库目录 + 解释器前缀。

    venv（sys.prefix，site-packages）与 uv/pyenv 管理的解释器
    （sys.base_prefix）常在 /usr 之外，必须显式挂载。按挂载目标路径字面
    去重（**不能**按 realpath：/lib64→usr/lib 这类符号链接若按 realpath
    去重，沙箱里就没有 /lib64 路径，动态加载器直接 ENOENT）。
    **不含 /data、/etc**。
    """
    args: List[str] = []
    seen: set = set()
    for path in ("/usr", "/lib", "/lib64", "/bin", "/sbin",
                 sys.base_prefix, sys.prefix):
        if os.path.isdir(path) and path not in seen:
            seen.add(path)
            args.extend(["--ro-bind", path, path])
    return args


def _probe_cmd() -> List[str]:
    """后端探测命令：与真跑同一挂载形态（W-0048 裁决点② 探针保真）。

    旧探针缺 ``--tmpfs /tmp``：venv/flows 落在 /tmp 下时探针通过但真跑
    execvp ENOENT（tmpfs 遮蔽），后端误判 bwrap 可用。探针与
    ``sandbox_cmd`` 共享 ``_runtime_ro_binds`` + tmpfs/proc/dev/unshare-net
    骨架（tmpfs 同样先于 bind，与真跑顺序一致）；flows/workdir 挂载与
    具体 run 相关，探针不复制。
    """
    bwrap = shutil.which("bwrap")
    if not bwrap:
        return []
    return [
        bwrap,
        "--tmpfs", "/tmp",
        *_runtime_ro_binds(),
        "--dev", "/dev",
        "--proc", "/proc",
        "--unshare-net",
        "--", sys.executable, "-c", "pass",
    ]


def parse_backend_choice(raw: str) -> str:
    """SANDBOX_BACKEND env → 合法后端选择；非法取值 fail-fast（ValueError）。"""
    if raw not in BACKEND_CHOICES:
        raise ValueError(
            f"SANDBOX_BACKEND 非法取值 {raw!r}（可选 {'/'.join(BACKEND_CHOICES)}）"
        )
    return raw


def _probe_bwrap() -> bool:
    """bwrap 可用性探针（环境事实，进程内缓存）。

    缓存只存探测结果——后端**选择**由 ``RunSpec.backend``（Settings 的
    ``sandbox_backend``）显式给出，与本缓存无关。测试经 ``_PROBE_OK = None``
    重置（配合 PATH 遮蔽模拟无 bwrap 环境）。
    """
    global _PROBE_OK
    if _PROBE_OK is None:
        _PROBE_OK = False
        probe = _probe_cmd()
        if probe:
            try:
                subprocess.run(probe, check=True, capture_output=True, timeout=10)
                _PROBE_OK = True
            except (subprocess.SubprocessError, OSError):
                _PROBE_OK = False
        if _PROBE_OK:
            logger.info("script sandbox probe: bwrap 可用（按需挂载 + unshare-net）")
        else:
            logger.warning(
                "script sandbox probe: bwrap 不可用（auto 后端将 fail-closed 503）"
            )
    return _PROBE_OK


def detect_backend() -> str:
    """环境探测：bwrap 可用 → "bwrap"，否则 "rlimit"（skip 守卫/运维观测用）。

    运行期后端选择不走这里——由 Settings.sandbox_backend 经
    ``effective_backend``/``verify_sandbox_backend`` 显式解析。
    """
    return BACKEND_BWRAP if _probe_bwrap() else BACKEND_RLIMIT


def effective_backend(spec: RunSpec) -> str:
    """Settings.sandbox_backend → 有效后端（可用性把关归 verify_sandbox_backend）。

    - ``rlimit``：显式测试选择，直接用。
    - ``bwrap``：显式选择，直接用（不可用由 verify 503，不静默降级）。
    - ``auto``（默认）：探测决定。
    """
    if spec.backend == BACKEND_RLIMIT:
        return BACKEND_RLIMIT
    if spec.backend == BACKEND_BWRAP:
        return BACKEND_BWRAP
    return detect_backend()


def verify_sandbox_backend(spec: RunSpec) -> None:
    """fail-closed 把关（W-0048 T2）：生产只有 bwrap 一条路径。

    - 显式 ``rlimit``（测试夹具语义）：放行——rlimit 不隔离网络与沙箱外
      FS，仅限测试，选择本身即显式。
    - 显式 ``bwrap`` 但探测失败：503（显式选择不静默降级）。
    - ``auto`` 探测不到 bwrap：503。旧 ``ALLOW_RLIMIT_FALLBACK=1`` env
      放行语义已删除——生产 env 不再影响判定。
    检查只做判断，无副作用。
    """
    if spec.backend == BACKEND_RLIMIT:
        logger.warning(
            "sandbox_backend=rlimit（显式选择，仅测试）：网络与沙箱外文件系统不隔离"
        )
        return
    if _probe_bwrap():
        return
    if spec.backend == BACKEND_BWRAP:
        raise HTTPException(
            status_code=503,
            detail="SANDBOX_BACKEND=bwrap 但本机 bwrap 不可用（probe 失败）；"
            "安装 bubblewrap 或改用 auto",
        )
    raise HTTPException(
        status_code=503,
        detail="沙箱后端 bwrap 不可用（probe 失败），run/save fail-closed："
        "生产请安装 bubblewrap；rlimit 降级不隔离网络与沙箱外文件系统，"
        "仅测试可经 SANDBOX_BACKEND=rlimit 显式选择",
    )


def sandbox_env(spec: RunSpec, workdir: str) -> Dict[str, str]:
    """沙箱环境变量：PYTHONPATH = flows_dir + extra_pythonpath（冒号分隔）。"""
    pythonpath = spec.flows_dir
    if spec.extra_pythonpath:
        pythonpath = pythonpath + ":" + ":".join(spec.extra_pythonpath)
    return {
        "PATH": os.environ.get("PATH", "/usr/local/bin:/usr/bin:/bin"),
        "PYTHONPATH": pythonpath,
        "PYTHONDONTWRITEBYTECODE": "1",
        "PYTHONUTF8": "1",
        # BLAS thread pools blow the 1 GiB address-space limit with per-thread
        # stacks; scripts are single-threaded geometry code anyway.
        "OPENBLAS_NUM_THREADS": "1",
        "OMP_NUM_THREADS": "1",
        "MKL_NUM_THREADS": "1",
        "NUMEXPR_NUM_THREADS": "1",
        "HOME": workdir,
        "TMPDIR": workdir,
    }


def _ro_bind_targets(args: List[str]) -> set:
    """从扁平 bwrap 参数取 --ro-bind 的挂载目标集合（bind 是三元组）。"""
    targets: set = set()
    i = 0
    while i < len(args) - 2:
        if args[i] == "--ro-bind":
            targets.add(args[i + 2])
            i += 3
        else:
            i += 1
    return targets


def sandbox_cmd(
    spec: RunSpec, workdir: str, extra_binds: Tuple[str, ...] = ()
) -> List[str]:
    """Wrap the command in bwrap when available (按需挂载 + no network).

    挂载集：系统库/解释器（``_runtime_ro_binds``）+ flows_dir 只读 +
    extra_ro_binds（存在的目录）只读 + extra_binds（T4 依赖 env 目录与
    base 解释器前缀，与已有挂载按目标路径去重）+ tmpfs /tmp + 最小 /dev
    + /proc + 可写 workdir。**不挂 /data、/etc**。

    挂载顺序：``--tmpfs /tmp`` 必须先于一切 bind——父挂载先于子挂载，
    否则落在 /tmp 下的 venv/flows/env 目录被 tmpfs 遮蔽（execvp ENOENT，
    与 workdir bind 一直在 tmpfs 之后同理）。
    """
    if effective_backend(spec) == BACKEND_BWRAP:
        runtime = _runtime_ro_binds()
        seen = _ro_bind_targets(runtime)
        binds: List[str] = []
        for d in (spec.flows_dir, *spec.extra_ro_binds, *extra_binds):
            if d and os.path.isdir(d) and d not in seen:
                seen.add(d)
                binds += ["--ro-bind", d, d]
        return [
            shutil.which("bwrap") or "bwrap",
            "--tmpfs", "/tmp",
            *runtime,
            *binds,
            "--dev", "/dev",
            "--proc", "/proc",
            "--bind", workdir, workdir,
            "--chdir", workdir,
            "--unshare-net",
            "--die-with-parent",
            "--",
        ]
    return []
