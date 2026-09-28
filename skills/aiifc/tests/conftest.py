"""aiifc tests conftest——注入 aiifc 包 + aidxf dxfkit/archdxf（consume_upstream 的 DXF 读取依赖）。"""

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]          # skills/aiifc
REPO = ROOT.parent.parent                            # repo root

# aiifc 包（scripts/aiifc_cli）
sys.path.insert(0, str(ROOT / "scripts" / "aiifc_cli"))
# aidxf dxfkit/archdxf（consume_upstream 的 DXF outline 读取依赖）。
# 单一事实源 = 源目录（skills/aidxf，git 跟踪，CI 干净克隆可用）；dist 是
# gitignored 打包产物，存在时优先（与 services/cad drawlib / W-0052 regen.py
# 的 bootstrap 顺序一致），缺失回退源（W-0053：修复 dist 未构建时 DXF 用例
# 全体 ModuleNotFoundError 的漂移）。
for pkg in ("dxfkit", "archdxf"):
    for base in (REPO / "skills" / "dist" / "aidxf" / "scripts" / "packages",
                 REPO / "skills" / "aidxf" / "scripts" / "packages"):
        p = base / pkg / "src"
        if p.is_dir():
            sys.path.insert(0, str(p))
            break
