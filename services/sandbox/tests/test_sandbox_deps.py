# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""test_sandbox_deps.py — W-0048 T4 契约：PEP 723 依赖声明 + uv 内容寻址环境。

钉死：

- 声明解析：无块 → None（走默认集）；多行数组/注释；块内无 dependencies
  键 → 显式空集；非法 TOML → ValueError（ensure_env 翻 422）。
- 依赖集选择：声明**替换**默认集（PEP 723 语义，不是合并）；无声明注入
  服务默认集（适配器声明，存量脚本基线）。
- 缓存 key：deps 序无关、集敏感、含 python tag。
- 缓存根：默认 ``$XDG_CACHE_HOME/aibim-sandbox-envs``（或 ~/.cache），
  ``SANDBOX_ENV_CACHE_DIR`` 可配——**不放 data/**（bwrap 故意不挂 /data）。
- env 构建/复用：命中缓存不重复调 uv（``_run_uv`` seam 计数）；声明依赖
  沙箱内可 import（e2e）；bwrap 命令含 env 目录 + base 解释器 home 的
  --ro-bind（去重）。
- 失败模式：依赖不存在 → 422；断网/离线（UV_OFFLINE=1）→ 503；
  uv 缺失 → 503（明确报错，不静默回退）。

真跑 uv 的用例需要网络与 uv 二进制（CI 三 job 均有）；uv 缺失整组 skip。
"""

from __future__ import annotations

import json
import os
import shutil
import sys
from pathlib import Path

import pytest
from fastapi import HTTPException

from aibim_sandbox import deps as deps_mod

UV_AVAILABLE = shutil.which("uv") is not None
requires_uv = pytest.mark.skipif(
    not UV_AVAILABLE, reason="uv 不可用（T4 依赖解析需要 uv）"
)

TINY_DEP = "six"  # 纯 wheel 小包（py2.py3-none-any），--only-binary 友好
NONEXISTENT_DEP = "aibim-nonexistent-pkg-xyz-404"


def _declared_script(dep_specs, module=None):
    """PEP 723 头 + 最小契约脚本；module 非空时 build 内 import 该模块。"""
    dep_lines = "\n".join(f"#     {json.dumps(d)}," for d in dep_specs)
    import_line = f"    import {module}\n" if module else ""
    marker = module if module else "none"
    return (
        "# /// script\n"
        '# requires-python = ">=3.10"\n'
        "# dependencies = [\n"
        f"{dep_lines}\n"
        "# ]\n"
        "# ///\n"
        "\n"
        'PARAMS = {"a": 1}\n'
        "\n"
        "def build(params, out_path):\n"
        f"{import_line}"
        '    with open(out_path, "w", encoding="utf-8") as fh:\n'
        f'        fh.write("DEPS-OK:{marker}")\n'
        "\n"
        'if __name__ == "__main__":\n'
        "    import sys\n"
        "    build(PARAMS, sys.argv[1])\n"
    )


class TestPep723Parse:
    """PEP 723 头解析（纯函数，不起子进程）。"""

    def test_no_block_returns_none(self, adapter):
        assert adapter.runner.parse_pep723_deps('PARAMS = {"a": 1}\n') is None

    def test_multi_line_array_with_comments(self, adapter):
        script = _declared_script(["six", "packaging"])
        assert adapter.runner.parse_pep723_deps(script) == ("six", "packaging")

    def test_block_without_dependencies_is_explicit_empty(self, adapter):
        script = "# /// script\n# requires-python = \">=3.10\"\n# ///\nPARAMS = {}\n"
        assert adapter.runner.parse_pep723_deps(script) == ()

    def test_malformed_toml_raises_value_error(self, adapter):
        script = "# /// script\n# dependencies = [\n# ///\nPARAMS = {}\n"
        with pytest.raises(ValueError):
            adapter.runner.parse_pep723_deps(script)

    def test_declared_replaces_default_set(self, adapter, settings):
        """声明即全量（替换默认集，不合并）——PEP 723 语义。"""
        script = _declared_script(["six"])
        assert adapter.runner.deps_for_script(settings, script) == ("six",)

    def test_no_block_uses_service_default_set(self, adapter, settings):
        """存量脚本（无声明）注入服务默认集（适配器声明的两侧差异）。"""
        deps = adapter.runner.deps_for_script(settings, 'PARAMS = {"a": 1}\n')
        assert tuple(deps) == adapter.default_deps


class TestEnvKey:
    """内容寻址 key：deps 排序无关、集合敏感、含 python tag。"""

    def test_order_insensitive(self, adapter):
        assert adapter.runner.env_key(("six", "packaging")) == adapter.runner.env_key(
            ("packaging", "six")
        )

    def test_set_sensitive(self, adapter):
        assert adapter.runner.env_key(("six",)) != adapter.runner.env_key(("ezdxf",))

    def test_python_tag_sensitive(self, adapter):
        assert adapter.runner.env_key(("six",), python_tag="cpython-3.10") != (
            adapter.runner.env_key(("six",), python_tag="cpython-3.11")
        )


class TestCacheRoot:
    """缓存根：默认 XDG/~/.cache 下，env 可覆盖；绝不在 data/ 下。"""

    def test_default_under_xdg_cache(self, adapter, monkeypatch):
        monkeypatch.delenv("SANDBOX_ENV_CACHE_DIR", raising=False)
        monkeypatch.setenv("XDG_CACHE_HOME", "/xdg-test")
        root = adapter.runner.cache_root()
        assert root == os.path.join("/xdg-test", "aibim-sandbox-envs")
        assert "data" not in Path(root).parts

    def test_env_override_wins(self, adapter, monkeypatch, tmp_path):
        monkeypatch.setenv("SANDBOX_ENV_CACHE_DIR", str(tmp_path))
        assert adapter.runner.cache_root() == str(tmp_path)


@requires_uv
class TestEnsureEnv:
    """uv 内容寻址环境：构建、复用、失败模式（真跑 uv，需要网络）。"""

    @pytest.fixture()
    def cache_dir(self, tmp_path, monkeypatch):
        """每用例独立缓存根（hermetic；uv 自身 wheel 缓存仍全局复用）。"""
        root = tmp_path / "envs"
        monkeypatch.setenv("SANDBOX_ENV_CACHE_DIR", str(root))
        return root

    def test_builds_env_for_declared_deps(self, adapter, settings, cache_dir):
        env = adapter.runner.ensure_env(settings, _declared_script([TINY_DEP]))
        assert env.deps == (TINY_DEP,)
        assert Path(env.python).is_file()
        assert Path(env.env_dir).parent == cache_dir
        assert Path(env.base_home).is_dir()
        assert (Path(env.env_dir) / "pyvenv.cfg").is_file()

    def test_cache_hit_does_not_reinvoke_uv(
        self, adapter, settings, cache_dir, monkeypatch
    ):
        script = _declared_script([TINY_DEP])
        first = adapter.runner.ensure_env(settings, script)

        def _boom(argv, timeout):  # pragma: no cover - 命中即不应被调
            raise AssertionError(f"缓存命中却调了 uv: {argv}")

        monkeypatch.setattr(deps_mod, "_run_uv", _boom)
        second = adapter.runner.ensure_env(settings, script)
        assert second.env_dir == first.env_dir
        assert second.python == first.python

    def test_nonexistent_dep_422(self, adapter, settings, cache_dir):
        with pytest.raises(HTTPException) as exc:
            adapter.runner.ensure_env(
                settings, _declared_script([NONEXISTENT_DEP])
            )
        assert exc.value.status_code == 422
        assert "依赖" in str(exc.value.detail)
        # 失败不留半成品 env（下次重试仍走构建）
        key = adapter.runner.env_key((NONEXISTENT_DEP,))
        assert not (cache_dir / key / "pyvenv.cfg").exists()

    def test_offline_503(self, adapter, settings, cache_dir, monkeypatch):
        """断网模拟（UV_OFFLINE=1，uv 自身离线开关）→ 503 网络语义。"""
        monkeypatch.setenv("UV_OFFLINE", "1")
        with pytest.raises(HTTPException) as exc:
            adapter.runner.ensure_env(
                settings, _declared_script(["six==99.99.99"])
            )
        assert exc.value.status_code == 503
        assert "网络" in str(exc.value.detail)

    def test_uv_missing_503(self, adapter, settings, cache_dir, monkeypatch):
        """uv 缺失 → 503 明确报错（不静默回退到服务 venv）。"""
        monkeypatch.setenv("PATH", str(cache_dir))  # 遮蔽 uv
        with pytest.raises(HTTPException) as exc:
            adapter.runner.ensure_env(settings, _declared_script([TINY_DEP]))
        assert exc.value.status_code == 503
        assert "uv" in str(exc.value.detail)


@requires_uv
class TestDeclaredDepsEndToEnd:
    """声明依赖经 uv 解析后在沙箱内可 import（验收标准原文）。"""

    def test_declared_dep_importable_in_sandbox(
        self, adapter, settings, tmp_path, monkeypatch
    ):
        monkeypatch.setenv("SANDBOX_ENV_CACHE_DIR", str(tmp_path / "envs"))
        out = tmp_path / adapter.product_name
        adapter.runner.run_script(
            settings, _declared_script([TINY_DEP], module="six"), str(out)
        )
        assert out.read_text(encoding="utf-8") == "DEPS-OK:six"


class TestEnvBwrapBinds:
    """bwrap 挂载：env 目录 + base 解释器 home 只读进沙箱（去重），断网不变。"""

    def _fake_env(self, tmp_path):
        env_dir = tmp_path / "envs" / "k"
        base_home = tmp_path / "base"
        env_dir.mkdir(parents=True)
        base_home.mkdir()
        return env_dir, base_home

    def test_env_dirs_ro_bound(self, adapter, settings, tmp_path):
        env_dir, base_home = self._fake_env(tmp_path)
        env = adapter.runner.DepsEnv(
            python=str(env_dir / "bin" / "python"),
            env_dir=str(env_dir),
            base_home=str(base_home),
            deps=(TINY_DEP,),
        )
        rs = adapter.with_backend(settings, "bwrap")
        cmd = adapter.runner._sandbox_cmd(rs, str(tmp_path), extra_binds=env.ro_binds)
        cmdstr = " ".join(cmd)
        assert f"--ro-bind {env_dir} {env_dir}" in cmdstr
        assert f"--ro-bind {base_home} {base_home}" in cmdstr
        assert "--unshare-net" in cmd
        # --tmpfs /tmp 必须先于 bind（父挂载先于子挂载），否则 /tmp 下的
        # env 目录被 tmpfs 遮蔽（execvp ENOENT，T4 e2e 实证）
        assert cmd.index("--tmpfs") < cmd.index(str(env_dir))

    def test_base_home_dedup_with_runtime_binds(self, adapter, settings, tmp_path):
        """base_home 与运行时挂载（sys.base_prefix）重合时不重复 bind。"""
        env_dir, _ = self._fake_env(tmp_path)
        env = adapter.runner.DepsEnv(
            python=str(env_dir / "bin" / "python"),
            env_dir=str(env_dir),
            base_home=sys.base_prefix,
            deps=(TINY_DEP,),
        )
        rs = adapter.with_backend(settings, "bwrap")
        cmd = adapter.runner._sandbox_cmd(rs, str(tmp_path), extra_binds=env.ro_binds)
        occurrences = sum(
            1 for i, tok in enumerate(cmd[:-1])
            if tok == "--ro-bind" and cmd[i + 1] == sys.base_prefix
        )
        assert occurrences == 1
