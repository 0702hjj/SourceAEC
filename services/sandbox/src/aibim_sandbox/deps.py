# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""deps.py — PEP 723 依赖声明 + uv 内容寻址执行环境（W-0048 T4）。

分层（用户裁决 2026-08-19）：uv 管依赖隔离/可复现，bwrap 管安全边界
（FS/网络/资源），两者组合而非替代。

- **声明**：脚本头部 ``# /// script`` 块（PEP 723 TOML）的
  ``dependencies``；**声明即全量**（替换服务默认集，不合并）。无声明的
  存量脚本注入 ``SandboxConfig.default_deps``（ifc：ifcopenshell+numpy；
  cad：ezdxf——两侧模板/测试脚本的实际白嫖清单）。
- **宿主机解析、沙箱执行**：父进程（有网）``uv venv`` + ``uv pip install
  --only-binary :all:``（不执行 sdist 构建代码——供应链约束），子进程
  用 env 的解释器起脚本，沙箱 ``--unshare-net`` 不变。
- **内容寻址缓存**：key = sha256(排序后 deps + python 实现/主次版本/
  平台)，命中即复用不重复解析；构建在临时目录完成后 ``os.rename``
  原子发布（并发构建输家复用赢家成果）。
- **缓存目录**：默认 ``$XDG_CACHE_HOME/aibim-sandbox-envs``（或
  ``~/.cache/``），``SANDBOX_ENV_CACHE_DIR`` 可配（两服务同名同义）——
  **不放 data/**（bwrap 故意不挂 /data，W-0047 跨租户读洞）。
- **失败语义**：uv 缺失 → 503（部署问题，明确报错不静默回退）；依赖
  解析失败（包不存在）→ 422（脚本声明问题，带 uv stderr 截尾）；网络
  不可达/离线（``UV_OFFLINE=1`` 可模拟）→ 503。
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
import re
import shutil
import subprocess
import sys
from dataclasses import dataclass
from typing import List, Optional, Tuple

from fastapi import HTTPException

from .spec import RunSpec

try:  # Python 3.11+ 标准库；3.10 走 tomli（pyproject 条件依赖）
    import tomllib
except ModuleNotFoundError:  # pragma: no cover - 3.10 环境
    import tomli as tomllib

logger = logging.getLogger(__name__)

ENV_BUILD_TIMEOUT_S = 300  # uv 解析+安装上限（env SCRIPT_ENV_BUILD_TIMEOUT_S）

# uv stderr 里的网络不可达标记（→503）；其余解析失败 → 422。
_NETWORK_MARKERS = (
    "error sending request",
    "dns error",
    "failed to connect",
    "connection refused",
    "network is unreachable",
    "network connectivity",
    "temporary failure",
    "offline",
    "timed out",
)

_BLOCK_START = re.compile(r"(?m)^# /// script\s*$")
_BLOCK_END = re.compile(r"^# ///\s*$")


@dataclass(frozen=True)
class DepsEnv:
    """uv 内容寻址执行环境（bwrap ro-bind + 子进程解释器）。"""

    python: str  # env 内解释器绝对路径（env_dir/bin/python）
    env_dir: str  # 内容寻址缓存目录（bwrap --ro-bind）
    base_home: str  # base 解释器前缀（venv symlink 目标，pyvenv.cfg home 推导）
    deps: Tuple[str, ...]  # 生效依赖集（脚本声明或服务默认集）

    @property
    def ro_binds(self) -> Tuple[str, ...]:
        return (self.env_dir, self.base_home)


def parse_pep723_deps(script_text: str) -> Optional[Tuple[str, ...]]:
    """PEP 723 ``# /// script`` 块 → dependencies tuple；无块 → None（走默认集）。

    块在但无 ``dependencies`` 键 → ``()``（显式空集：用户明确不要任何包，
    也不注入默认集）。TOML 非法/dependencies 非字符串数组 → ValueError。
    """
    match = _BLOCK_START.search(script_text)
    if not match:
        return None
    lines: List[str] = []
    for line in script_text[match.end():].splitlines():
        if _BLOCK_END.match(line):
            break
        if not line.strip():
            continue  # 起始行残余空串 / 块内空行（对 TOML 无贡献）
        if not line.startswith("#"):
            raise ValueError(f"PEP 723 块含非注释行: {line!r}")
        content = line[1:]
        if content.startswith(" "):
            content = content[1:]
        lines.append(content)
    data = tomllib.loads("\n".join(lines))  # TOMLDecodeError 是 ValueError 子类
    deps = data.get("dependencies", [])
    if not isinstance(deps, list) or not all(isinstance(d, str) for d in deps):
        raise ValueError("PEP 723 dependencies 必须是字符串数组")
    return tuple(deps)


def deps_for_script(spec: RunSpec, script_text: str) -> Tuple[str, ...]:
    """脚本生效依赖集：PEP 723 声明优先（替换），无声明用服务默认集。"""
    declared = parse_pep723_deps(script_text)
    if declared is not None:
        return declared
    return spec.config.default_deps


def _python_tag() -> str:
    v = sys.version_info
    return f"{sys.implementation.name}-{v.major}.{v.minor}-{sys.platform}"


def env_key(deps: Tuple[str, ...], python_tag: Optional[str] = None) -> str:
    """内容寻址 key：deps 排序无关、集合敏感，绑定 python 实现/版本/平台。"""
    payload = json.dumps(
        {
            "deps": sorted(d.strip() for d in deps),
            "python": python_tag if python_tag is not None else _python_tag(),
        },
        ensure_ascii=False,
        sort_keys=True,
    )
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:16]


def cache_root() -> str:
    """env 缓存根：SANDBOX_ENV_CACHE_DIR 优先，默认 XDG cache 下。不放 data/。"""
    override = os.environ.get("SANDBOX_ENV_CACHE_DIR")
    if override:
        return override
    xdg = os.environ.get("XDG_CACHE_HOME") or os.path.join(
        os.path.expanduser("~"), ".cache"
    )
    return os.path.join(xdg, "aibim-sandbox-envs")


def _venv_python(env_dir: str) -> str:
    return os.path.join(env_dir, "bin", "python")


def _env_ready(env_dir: str) -> bool:
    """env 可用判定：pyvenv.cfg + bin/python 都在（原子发布保证无半成品）。"""
    return os.path.isfile(os.path.join(env_dir, "pyvenv.cfg")) and os.path.isfile(
        _venv_python(env_dir)
    )


def _base_home(env_dir: str) -> str:
    """pyvenv.cfg home → base 解释器**前缀**（venv symlink 目标，bwrap 需挂）。

    home 指向 base 的 bin 目录（如 ``<prefix>/bin``）；stdlib/libpython 在
    前缀下，故取 realpath 后若为 bin 则回退到其父目录。
    """
    with open(os.path.join(env_dir, "pyvenv.cfg"), encoding="utf-8") as fh:
        for line in fh:
            if line.startswith("home"):
                home = os.path.realpath(line.split("=", 1)[1].strip())
                if os.path.basename(home) == "bin":
                    return os.path.dirname(home)
                return home
    raise HTTPException(  # pragma: no cover - uv venv 必写 home
        status_code=503, detail=f"依赖环境 {env_dir} 的 pyvenv.cfg 缺 home 行"
    )


def _run_uv(argv: List[str], timeout: int) -> subprocess.CompletedProcess:
    """uv 子进程调用（测试 seam：缓存命中断言不重复解析）。"""
    return subprocess.run(argv, capture_output=True, timeout=timeout)


def _build_timeout() -> int:
    raw = os.environ.get("SCRIPT_ENV_BUILD_TIMEOUT_S")
    if raw and raw.isdigit() and int(raw) > 0:
        return int(raw)
    return ENV_BUILD_TIMEOUT_S


def _raise_env_build_error(stage: str, proc: subprocess.CompletedProcess) -> None:
    """uv 失败翻译：网络标记 → 503；其余（依赖不存在等解析失败）→ 422。"""
    tail = proc.stderr.decode("utf-8", errors="replace")[-2048:].strip()
    low = tail.lower()
    if any(marker in low for marker in _NETWORK_MARKERS):
        raise HTTPException(
            status_code=503,
            detail=f"脚本依赖环境构建失败（{stage}）：网络不可达或 uv 离线——{tail}",
        )
    raise HTTPException(
        status_code=422,
        detail=f"脚本声明的依赖无法解析（{stage}）：{tail}",
    )


def _build_env(target: str, deps: Tuple[str, ...]) -> None:
    """uv venv + uv pip install 构建 env，os.rename 原子发布到 target。"""
    uv = shutil.which("uv")
    if not uv:
        raise HTTPException(
            status_code=503,
            detail="脚本依赖解析需要 uv，但 PATH 中找不到 uv（T4 起 run/save "
            "依赖 uv 构建执行环境）；安装 uv 后重试",
        )
    timeout = _build_timeout()
    tmp = f"{target}.tmp-{os.getpid()}"
    shutil.rmtree(tmp, ignore_errors=True)
    os.makedirs(os.path.dirname(target), exist_ok=True)
    try:
        proc = _run_uv([uv, "venv", "--python", sys.executable, tmp], timeout)
        if proc.returncode != 0:
            _raise_env_build_error("uv venv", proc)
        if deps:
            proc = _run_uv(
                [uv, "pip", "install", "--python", _venv_python(tmp),
                 "--only-binary", ":all:", *deps],
                timeout,
            )
            if proc.returncode != 0:
                _raise_env_build_error("uv pip install", proc)
        logger.info("sandbox deps env built: %s (deps=%s)", target, list(deps))
        try:
            os.rename(tmp, target)  # 同目录 rename，原子发布
        except OSError:
            # 并发构建：别的 worker 先落地——复用赢家成果
            if not _env_ready(target):
                raise HTTPException(
                    status_code=503,
                    detail="依赖环境并发构建冲突且目标不可用，请重试",
                )
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def ensure_env(spec: RunSpec, script_text: str) -> DepsEnv:
    """脚本依赖（PEP 723 声明或服务默认集）→ 可用的 uv 环境（缓存命中复用）。

    422：PEP 723 声明非法 / 依赖无法解析；503：uv 缺失 / 网络不可达。
    """
    try:
        deps = deps_for_script(spec, script_text)
    except ValueError as exc:
        raise HTTPException(
            status_code=422, detail=f"PEP 723 依赖声明解析失败: {exc}"
        )
    target = os.path.join(cache_root(), env_key(deps))
    if not _env_ready(target):
        _build_env(target, deps)
    if not _env_ready(target):  # pragma: no cover - 并发冲突兜底
        raise HTTPException(
            status_code=503,
            detail="依赖环境构建后不可用（pyvenv.cfg/bin/python 缺失），请重试",
        )
    return DepsEnv(
        python=_venv_python(target),
        env_dir=target,
        base_home=_base_home(target),
        deps=deps,
    )
