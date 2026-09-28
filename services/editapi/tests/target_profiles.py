# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""TargetProfile——目标服务差异面的定义与实现（W-0060 T2 自 conftest 拆出）。

职责单一：目标解析（env/cwd → ifc|cad）+ sys.path 装配 + 两侧差异的全部
实现（脚本模板/目录构造/产物种子/语义读取）+ ``TargetProfile`` 声明与
``PROFILES`` 取值。conftest.py 只留夹具与样例物化，经再导出保持测试文件
``from conftest import ...`` 的导入面不变。

venv 互斥纪律：两侧实现（ifcopenshell / ezdxf+cad_script_lib）一律函数内
延迟 import，模块导入期不触碰目标服务依赖。
"""

from __future__ import annotations

import os
import shutil
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Callable

from script_runner_scripts import REAL_CAD_SCRIPT, REAL_IFC_SCRIPT

TESTS_DIR = Path(__file__).resolve().parent
EDITAPI_ROOT = TESTS_DIR.parent
SERVICES_ROOT = EDITAPI_ROOT.parent
REPO_ROOT = SERVICES_ROOT.parent

MODEL_ID = "m_0123456789abcdef"
UNKNOWN_MODEL_ID = "m_ffffffffffffffff"

VALID_TARGETS = ("ifc", "cad")

_RUN_HINT = (
    "在目标服务目录下运行（各用其 venv）：\n"
    "  cd services/ifc && uv run --group dev pytest ../editapi/tests -q\n"
    "  cd services/cad && uv run --group dev pytest ../editapi/tests -q\n"
    "或显式指定目标：EDITAPI_TARGET=ifc|cad"
)


def _resolve_target() -> str:
    env_target = os.environ.get("EDITAPI_TARGET", "").strip().lower()
    if env_target:
        if env_target not in VALID_TARGETS:
            raise RuntimeError(
                f"EDITAPI_TARGET={env_target!r} 无效（可选 {VALID_TARGETS}）。\n{_RUN_HINT}"
            )
        return env_target
    cwd_name = Path.cwd().name
    if cwd_name in VALID_TARGETS and (SERVICES_ROOT / cwd_name).is_dir():
        return cwd_name
    raise RuntimeError(
        f"无法从 cwd={Path.cwd()} 推导契约套件目标服务。\n{_RUN_HINT}"
    )


TARGET = _resolve_target()
SERVICE_ROOT = SERVICES_ROOT / TARGET

# 本模块经 conftest 早于任何 app.* import 生效：先把服务根目录插进
# sys.path，契约测试与夹具里的 ``app.main`` / ``app.script_runner`` 才可解析。
if str(SERVICE_ROOT) not in sys.path:
    sys.path.insert(0, str(SERVICE_ROOT))

if TARGET == "cad":
    # cad 侧 diff 版本对构造需要 cad_script_lib（镜像 services/cad conftest）。
    _CAD_FLOWS = SERVICE_ROOT / "flows"
    if str(_CAD_FLOWS) not in sys.path:
        sys.path.insert(0, str(_CAD_FLOWS))


# ---------------------------------------------------------------------------
# 共享构建脚本样例（与两侧现有套件同构；产物为纯文本，两侧沙箱均可执行）
# ---------------------------------------------------------------------------

def staging_script(marker: str) -> str:
    """marker 脚本：满足契约门（PARAMS/build/__main__），产物为纯文本。

    不调用 flows（script_lib / cad_script_lib），因此 run 后不产 map
    sidecar——save 布局用例据此区分「无 map sidecar」形态。
    """
    return (
        f'PARAMS = {{"marker": "{marker}"}}\n'
        "\n"
        "def build(params, out_path):\n"
        "    open(out_path, 'w').write('EDITAPI:' + params['marker'])\n"
        "\n"
        'if __name__ == "__main__":\n'
        "    import sys\n"
        "    build(PARAMS, sys.argv[1])\n"
    )


FAILING_SCRIPT = (
    'PARAMS = {"a": 1}\n'
    "\n"
    "def build(params, out_path):\n"
    "    raise RuntimeError('editapi-contract-fail-marker')\n"
    "\n"
    'if __name__ == "__main__":\n'
    "    import sys\n"
    "    build(PARAMS, sys.argv[1])\n"
)

CONTRACT_VIOLATION_SCRIPT = "x = 1\n"

_IFC_KEY_SCRIPT = '''\
import sys

import ifcopenshell

from script_lib import attach_design_key, create_entity, create_skeleton, write_and_validate

PARAMS = {{"key": "{key}"}}

def build(params, out_path):
    model = ifcopenshell.file(schema="IFC4")
    body, _ = create_skeleton(model)
    w = create_entity(model, "IfcWall", key="{key}", name="W1")
    attach_design_key(model, w, "{key}")
    write_and_validate(model, out_path)

if __name__ == "__main__":
    build(PARAMS, sys.argv[1])
'''

_CAD_KEY_SCRIPT = '''\
import sys

import ezdxf

from cad_script_lib import add_entity, write_and_validate

PARAMS = {{"key": "{key}"}}

def build(params, out_path):
    doc = ezdxf.new("R2010")
    msp = doc.modelspace()
    add_entity(msp, "TEXT", key="{key}", text="W1", insert=(0, 0))
    write_and_validate(doc, out_path)

if __name__ == "__main__":
    build(PARAMS, sys.argv[1])
'''

_IFC_PARAMS_KEY_SCRIPT = '''\
import sys

import ifcopenshell

from script_lib import attach_design_key, create_entity, create_skeleton, write_and_validate

PARAMS = {{"key": "{key}"}}

def build(params, out_path):
    model = ifcopenshell.file(schema="IFC4")
    body, _ = create_skeleton(model)
    w = create_entity(model, "IfcWall", key=params["key"], name="W1")
    attach_design_key(model, w, params["key"])
    write_and_validate(model, out_path)

if __name__ == "__main__":
    build(PARAMS, sys.argv[1])
'''

_CAD_PARAMS_KEY_SCRIPT = '''\
import sys

import ezdxf

from cad_script_lib import add_entity, write_and_validate

PARAMS = {{"key": "{key}"}}

def build(params, out_path):
    doc = ezdxf.new("R2010")
    msp = doc.modelspace()
    add_entity(msp, "TEXT", key=params["key"], text="W1", insert=(0, 0))
    write_and_validate(doc, out_path)

if __name__ == "__main__":
    build(PARAMS, sys.argv[1])
'''


def _traced_variant(script_text: str, key: str) -> str:
    """key 实参从字面量改为拼接表达式 → origin=traced（两侧同构改法）。"""
    head, _, rest = key.partition(":")
    literal = f'key="{key}"'
    expression = f'key="{head}:" + "{rest}"'
    assert literal in script_text, "key 脚本模板必须含 key= 字面量实参"
    return script_text.replace(literal, expression)


# ---------------------------------------------------------------------------
# 两侧差异实现（延迟 import 目标服务依赖；data_dir/种子/语义读取构造）
# ---------------------------------------------------------------------------

def _make_ifc_data_dir(tmp_path: Path) -> Path:
    fixture = (
        REPO_ROOT / "converter" / "test" / "fixtures"
        / "wall-with-opening-and-window.ifc"
    )
    uploads = tmp_path / "uploads"
    uploads.mkdir()
    (uploads / f"{MODEL_ID}.ifc").write_bytes(fixture.read_bytes())
    return tmp_path


def _make_cad_data_dir(tmp_path: Path) -> Path:
    import ezdxf

    (tmp_path / "models").mkdir()
    uploads = tmp_path / "uploads"
    uploads.mkdir()
    doc = ezdxf.new("R2010")
    msp = doc.modelspace()
    msp.add_line((0, 0), (10, 0))
    msp.add_circle((5, 5), 2)
    doc.saveas(uploads / f"{MODEL_ID}.dxf")
    return tmp_path


def _ifc_locate_key(data_dir: Path, key: str) -> str:
    """ifc locate 入参是构件 guid：从 run 后的 uploads IFC 解析。"""
    import ifcopenshell

    model = ifcopenshell.open(str(data_dir / "uploads" / f"{MODEL_ID}.ifc"))
    return model.by_type("IfcWall")[0].GlobalId


def _identity_locate_key(data_dir: Path, key: str) -> str:
    """cad locate 入参就是 XDATA key 本身。"""
    return key


def _ifc_write_diff_versions(data_dir: Path) -> None:
    """手写 v1/v2 快照：同一面墙改名（镜像 ifc test_diff 的写法）。"""
    import ifcopenshell

    versions = data_dir / "models" / MODEL_ID / "versions"
    versions.mkdir(parents=True, exist_ok=True)
    uploads = data_dir / "uploads" / f"{MODEL_ID}.ifc"
    shutil.copy(uploads, versions / "v1.ifc")
    model = ifcopenshell.open(str(uploads))
    model.by_type("IfcWall")[0].Name = "editapi-renamed"
    model.write(str(versions / "v2.ifc"))


def _cad_write_diff_versions(data_dir: Path) -> None:
    """手写 v1/v2 快照：同 key LINE 终点变化（XDATA key 经 cad_script_lib）。"""
    import cad_script_lib
    import ezdxf

    versions = data_dir / "models" / MODEL_ID / "versions"
    versions.mkdir(parents=True, exist_ok=True)
    for name, end in (("v1", (10, 0)), ("v2", (12, 0))):
        cad_script_lib.reset_state()
        doc = ezdxf.new("R2010")
        msp = doc.modelspace()
        cad_script_lib.add_entity(msp, "LINE", key="0:line:1", start=(0, 0), end=end)
        doc.saveas(str(versions / f"{name}.dxf"))


def _ifc_read_entity_label(data_dir: Path, model_id: str) -> str:
    """读 uploads IFC 首墙 Name（edit-call 改写生效的语义级校验，W-0060 T2）。"""
    import ifcopenshell

    model = ifcopenshell.open(str(data_dir / "uploads" / f"{model_id}.ifc"))
    return model.by_type("IfcWall")[0].Name


def _cad_read_entity_label(data_dir: Path, model_id: str) -> str:
    """读 uploads DXF 首 TEXT 文本（edit-call 改写生效的语义级校验，W-0060 T2）。"""
    import ezdxf

    doc = ezdxf.readfile(str(data_dir / "uploads" / f"{model_id}.dxf"))
    return doc.modelspace().query("TEXT")[0].dxf.text


def _ifc_seed_product(tmp_path: Path) -> Path:
    """ifc 产物种子：converter fixture 字节拷贝（镜像 ifc conftest 的 ifc_path）。"""
    fixture = (
        REPO_ROOT / "converter" / "test" / "fixtures"
        / "wall-with-opening-and-window.ifc"
    )
    dst = tmp_path / "model.ifc"
    dst.write_bytes(fixture.read_bytes())
    return dst


def _cad_seed_product(tmp_path: Path) -> Path:
    """cad 产物种子：真 DXF（LINE+CIRCLE，XDATA key 经 cad_script_lib）。"""
    import cad_script_lib
    import ezdxf

    cad_script_lib.reset_state()
    doc = ezdxf.new("R2010")
    msp = doc.modelspace()
    cad_script_lib.add_entity(msp, "LINE", start=(0, 0), end=(10, 0))
    cad_script_lib.add_entity(msp, "CIRCLE", center=(5, 5), radius=2)
    dst = tmp_path / "model.dxf"
    doc.saveas(dst)
    return dst


def _ifc_verify_real_product(out: Path) -> None:
    """REAL IFC 构建产物校验：骨架聚合树实体存在（镜像 ifc 侧原断言）。"""
    import ifcopenshell

    model = ifcopenshell.open(str(out))
    assert model.by_type("IfcProject")
    assert model.by_type("IfcBuildingStorey")


def _cad_verify_real_product(out: Path) -> None:
    """REAL DXF 构建产物校验：一条 LINE（镜像 cad 侧原断言）。"""
    import ezdxf

    doc = ezdxf.readfile(str(out))
    assert len(doc.modelspace().query("LINE")) == 1


@dataclass(frozen=True)
class TargetProfile:
    """目标服务的已知差异面（差异进 profile，断言保持形状级）。"""

    name: str
    ext: str                              # 产物扩展名 ifc / dxf
    locate_query: str                     # locate 查询参数名 guid / key
    locate_resp_field: str                # locate 响应键字段 designKey / key
    edit_body_field: str                  # edit-call body 键字段 designKey / key
    entity_arg: str                       # key 脚本中可改写的字面量实参名
    default_key: str                      # key 脚本使用的构件 design key
    default_deps: tuple[str, ...]         # 无 PEP 723 声明脚本的默认依赖集
    real_script: str                      # REAL 构建（真实库产物）脚本
    real_map_key: str                     # REAL 构建产物 map 信封中的确定性 key
    verify_real_product: Callable[[Path], None]
    make_data_dir: Callable[[Path], Path]
    seed_product: Callable[[Path], Path]           # 产物种子文件（W-0060 T1）
    key_script: Callable[[str], str]
    params_key_script: Callable[[str], str]        # key 走 params 引用的变体（W-0060 T2）
    read_entity_label: Callable[[Path, str], str]  # uploads 产物实体标签读取（W-0060 T2）
    locate_key: Callable[[Path, str], str]
    write_diff_versions: Callable[[Path], None]


PROFILES: dict[str, TargetProfile] = {
    "ifc": TargetProfile(
        name="ifc",
        ext="ifc",
        locate_query="guid",
        locate_resp_field="designKey",
        edit_body_field="designKey",
        entity_arg="name",
        default_key="s1:wall:1",
        default_deps=("ifcopenshell>=0.8", "numpy"),
        real_script=REAL_IFC_SCRIPT,
        real_map_key="skeleton:project",
        verify_real_product=_ifc_verify_real_product,
        make_data_dir=_make_ifc_data_dir,
        seed_product=_ifc_seed_product,
        key_script=lambda key: _IFC_KEY_SCRIPT.format(key=key),
        params_key_script=lambda key: _IFC_PARAMS_KEY_SCRIPT.format(key=key),
        read_entity_label=_ifc_read_entity_label,
        locate_key=_ifc_locate_key,
        write_diff_versions=_ifc_write_diff_versions,
    ),
    "cad": TargetProfile(
        name="cad",
        ext="dxf",
        locate_query="key",
        locate_resp_field="key",
        edit_body_field="key",
        entity_arg="text",
        default_key="s1:text:1",
        default_deps=("ezdxf>=1.3",),
        real_script=REAL_CAD_SCRIPT,
        real_map_key="0:line:1",
        verify_real_product=_cad_verify_real_product,
        make_data_dir=_make_cad_data_dir,
        seed_product=_cad_seed_product,
        key_script=lambda key: _CAD_KEY_SCRIPT.format(key=key),
        params_key_script=lambda key: _CAD_PARAMS_KEY_SCRIPT.format(key=key),
        read_entity_label=_cad_read_entity_label,
        locate_key=_identity_locate_key,
        write_diff_versions=_cad_write_diff_versions,
    ),
}

PROFILE = PROFILES[TARGET]
