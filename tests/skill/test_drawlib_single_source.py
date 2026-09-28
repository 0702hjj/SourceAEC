"""W-0053：venv 与 dist 一致性检查——防 skills/.venv 旧 editable 漂移。

背景（2026-09-19 W-0053 评估实证）：
- `tools/install_skill_venv.sh` 从 `skills/dist/aidxf` editable 安装 archdxf/dxfkit，
  venv 的 `__editable__.*.pth` 钉在 **dist 副本**；
- services/cad 沙箱 drawlib（config.PROFILE.drawlib_repo_rel_paths）直接引用
  **源** `skills/aidxf/scripts/packages/*/src`。
改源后忘记重打包（skill_pack）+ 重装（install_skill_venv）→ venv 继续供旧码
（AI 生成 / cad→ifc 消化的 readback 都走它），人机沙箱读新源——双路径静默漂移
（item W-0053 所述「venv 旧 editable → dist 未同步 → unrecognized」事故类）。

检查语义：
1. `skills/.venv` 不存在 → skip（CI 干净克隆 / 未装 skill CLI）。
2. venv 存在但某包未以 editable 安装 → skip（安装面归 tools/install_skill_venv.sh，
   其修复在 W-0058）。
3. editable 目标目录不存在（指向已删/迁移路径）→ **fail**（正是 unrecognized 事故形态）。
4. editable 目标与本仓**源**包逐文件（*.py + py.typed）哈希不一致 → **fail**
   （改了源没重打包/重装）。

配套：dist↔源 的同步比对在 services/cad/tests/test_script_runner_drawlib.py
（test_dist_archdxf_in_sync_with_source，dist 存在时真比对）——本文件不重复。
"""

import hashlib
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SOURCE_PACKAGES = REPO_ROOT / "skills" / "aidxf" / "scripts" / "packages"
VENV_DIR = REPO_ROOT / "skills" / ".venv"
PACKAGES = ("archdxf", "dxfkit")
MARKER_FILES = ("py.typed",)


def _venv_site_packages() -> Path | None:
    """skills/.venv 的 site-packages（lib/python*/site-packages）；无 venv → None。"""
    if not VENV_DIR.is_dir():
        return None
    candidates = sorted(VENV_DIR.glob("lib/python*/site-packages"))
    return candidates[-1] if candidates else None


def _editable_target(site: Path, pkg: str) -> Path | None:
    """读 `__editable__.{pkg}-*.pth` 的目标路径；未安装 → None。

    src-layout editable 安装的 .pth 内容 = 一行绝对路径（指向 <pkg>/src）。
    """
    pths = sorted(site.glob(f"__editable__.{pkg}-*.pth"))
    if not pths:
        return None
    target = pths[0].read_text(encoding="utf-8").strip().splitlines()[0].strip()
    return Path(target) if target else None


def _pkg_files(pkg_dir: Path) -> dict[str, str]:
    """包目录 → {相对路径: 文件 sha256}（.py + py.typed；排除 __pycache__）。"""
    out: dict[str, str] = {}
    for f in sorted(pkg_dir.rglob("*")):
        if not f.is_file() or "__pycache__" in f.parts:
            continue
        if f.suffix == ".py" or f.name in MARKER_FILES:
            rel = f.relative_to(pkg_dir).as_posix()
            out[rel] = hashlib.sha256(f.read_bytes()).hexdigest()
    return out


class TestVenvDistConsistency(unittest.TestCase):
    """skills/.venv editable 安装的 archdxf/dxfkit 与源单一事实源一致。"""

    def setUp(self):
        site = _venv_site_packages()
        if site is None:
            self.skipTest("skills/.venv 不存在（未安装 skill CLI）")
        self.site = site

    def test_editable_targets_exist_and_in_repo(self):
        """editable 目标必须存在且在本仓内——指向已删/外仓路径即 unrecognized 形态。"""
        for pkg in PACKAGES:
            target = _editable_target(self.site, pkg)
            if target is None:
                self.skipTest(f"{pkg} 未以 editable 安装（安装面归 install_skill_venv.sh）")
            self.assertTrue(
                target.is_dir(),
                f"{pkg} editable 目标不存在（旧路径残留？）: {target}",
            )
            self.assertIn(
                str(REPO_ROOT), str(target.resolve()),
                f"{pkg} editable 目标不在本仓（复用了别处 venv？）: {target}",
            )

    def test_editable_content_matches_source(self):
        """editable 指向的包内容与源逐文件一致（哈希）——改源未重打包/重装即 fail。"""
        problems_all = []
        for pkg in PACKAGES:
            target = _editable_target(self.site, pkg)
            if target is None or not target.is_dir():
                self.skipTest(f"{pkg} 未有效安装（前一条用例负责报错形态）")
            src_dir = SOURCE_PACKAGES / pkg / "src" / pkg
            venv_files = _pkg_files(target / pkg)
            src_files = _pkg_files(src_dir)
            only_venv = sorted(set(venv_files) - set(src_files))
            only_src = sorted(set(src_files) - set(venv_files))
            diff = sorted(
                k for k in set(venv_files) & set(src_files)
                if venv_files[k] != src_files[k]
            )
            if only_venv or only_src or diff:
                problems_all.append(
                    f"{pkg}: 内容不一致（改源后未重打包+重装？）"
                    f" 仅venv={only_venv} 仅源={only_src} 有差异={diff}"
                )
        self.assertFalse(
            problems_all, "; ".join(problems_all) or "venv 与源包内容漂移"
        )


if __name__ == "__main__":
    unittest.main()
