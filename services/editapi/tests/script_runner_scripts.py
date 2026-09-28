# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_runner 测试共享的构建脚本样例（W-0057 T3 收编，非测试模块）。

原 services/ifc 与 services/cad 各自的 tests/script_runner_scripts.py 合一：
两侧字节级同构的样例直接保留为常量；payload/扩展名有差异的样例改为
builder（由 conftest 按 target 物化后供测试导入——本模块不解析 target，
保持无循环依赖）。

按用途分组：契约门样例（违规各形态）、happy path（真实产物构建，per-target
REAL 脚本随常量保留）、失败/恶意样例（运行时错误、死循环、内存炸弹、
越界写、输出洪泛、超大产物），供 test_script_runner_*.py 各领域模块复用。

payload 归一化说明：原 cad 侧 GOOD 用 ``0\\nSECTION\\n``、其余用
``0\\nSECTION``，ifc 侧统一 ``ISO-10303-21;``——payload 文本不参与任何
断言（断言只看产物存在性/``/* t */`` 标记），故收敛为每 target 一个
``PRODUCT_PAYLOADS`` 值。
"""

from __future__ import annotations

# conftest 按 TARGET 取值物化 GOOD/ESCAPE/FLOOD 等样例（ifc/cad 单点差异）。
PRODUCT_PAYLOADS: dict[str, str] = {
    "ifc": "ISO-10303-21;",
    "cad": "0\\nSECTION",
}

# ---------------------------------------------------------------------------
# 契约门样例（两侧字节级同构）
# ---------------------------------------------------------------------------

NO_PARAMS_SCRIPT = '''\
def build(params, out_path):
    open(out_path, "w").write("x")

if __name__ == "__main__":
    import sys
    build({}, sys.argv[1])
'''

NO_BUILD_SCRIPT = '''\
PARAMS = {"a": 1}

if __name__ == "__main__":
    pass
'''

NO_MAIN_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    open(out_path, "w").write("x")
'''

NON_LITERAL_PARAMS_SCRIPT = '''\
import os
PARAMS = {"a": os.environ.get("A")}

def build(params, out_path):
    open(out_path, "w").write("x")

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

# ---------------------------------------------------------------------------
# 失败/恶意/限额样例（两侧字节级同构）
# ---------------------------------------------------------------------------

RUNTIME_ERROR_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    raise RuntimeError("boom-marker")

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

INFINITE_LOOP_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    while True:
        pass

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

MEMORY_BOMB_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    blob = b"y" * (4 << 30)
    open(out_path, "wb").write(blob[:1])

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

NOISY_FAILURE_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    import sys
    sys.stderr.write("E" * 10000)
    sys.exit(3)

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

FORK_LOOP_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    import os, sys, time
    if os.fork() == 0:
        sys.stderr.write("CHILD:%d\\n" % os.getpid())
        sys.stderr.flush()
        while True:
            time.sleep(1)
    while True:
        time.sleep(1)

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

LEAK_PROBE_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    import os
    leaked = []
    for path in ("/data", "/etc/passwd"):
        try:
            if os.path.isfile(path):
                with open(path, "rb") as fh:
                    fh.read(1)
            else:
                os.listdir(path)
            leaked.append(path)
        except OSError:
            pass
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write(("LEAKED:" + ",".join(leaked)) if leaked else "BLOCKED")
        fh.write("|TMP:" + ("ok" if os.path.isdir("/tmp") else "missing"))

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

BIG_WRITE_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    with open(out_path, "wb") as fh:
        fh.write(b"X" * (4 << 20))

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

# ---------------------------------------------------------------------------
# payload/扩展名差异样例 builder（conftest 物化）
# ---------------------------------------------------------------------------

_GOOD_TEMPLATE = '''\
PARAMS = {"name": "t", "width": 6}

def build(params, out_path):
    with open(out_path, "w", encoding="utf-8") as fh:
        __WRITE_STMT__

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''


def good_write_stmt(payload: str) -> str:
    """GOOD 写语句的源文本（replace-based 改写用例的锚点，两侧同构）。"""
    return f'fh.write("{payload} /* " + params["name"] + " */")'


def good_script(payload: str) -> str:
    """合法契约脚本：写含 ``/* {name} */`` 标记的纯文本产物。"""
    return _GOOD_TEMPLATE.replace("__WRITE_STMT__", good_write_stmt(payload))


def no_output_script(ext: str) -> str:
    """忽略 argv[1]、写别处文件 → 422「未产出」。"""
    return (
        'PARAMS = {"a": 1}\n'
        "\n"
        "def build(params, out_path):\n"
        f'    open("elsewhere.{ext}", "w").write("x")  # ignores argv[1]\n'
        "\n"
        'if __name__ == "__main__":\n'
        "    import sys\n"
        "    build(PARAMS, sys.argv[1])\n"
    )


def escape_write_script(payload: str) -> str:
    """越界写样本：先写 params["target"]（测试注入沙箱外路径）再写产物。"""
    return (
        'PARAMS = {"a": 1}\n'
        "\n"
        "def build(params, out_path):\n"
        '    open(params["target"], "w").write("pwned")\n'
        f'    open(out_path, "w").write("{payload}")\n'
        "\n"
        'if __name__ == "__main__":\n'
        "    import sys\n"
        "    build(PARAMS, sys.argv[1])\n"
    )


def stdout_flood_script(payload: str) -> str:
    """stdout 洪泛 4MiB（超输出上限，进程组被杀）。"""
    return (
        'PARAMS = {"a": 1}\n'
        "\n"
        "def build(params, out_path):\n"
        "    import sys\n"
        '    sys.stdout.write("F" * (4 << 20))\n'
        "    sys.stdout.flush()\n"
        f'    open(out_path, "w").write("{payload}")\n'
        "\n"
        'if __name__ == "__main__":\n'
        "    import sys\n"
        "    build(PARAMS, sys.argv[1])\n"
    )


def moderate_stdout_script(payload: str) -> str:
    """100KB 中等输出（低于上限，正常跑完）。"""
    return (
        'PARAMS = {"a": 1}\n'
        "\n"
        "def build(params, out_path):\n"
        "    import sys\n"
        '    sys.stdout.write("O" * 100000)\n'
        "    sys.stdout.flush()\n"
        f'    open(out_path, "w").write("{payload}")\n'
        "\n"
        'if __name__ == "__main__":\n'
        "    import sys\n"
        "    build(PARAMS, sys.argv[1])\n"
    )


def big_map_script(payload: str) -> str:
    """产物合法但 map sidecar 超大（发布前拒绝，产物不落盘）。"""
    return (
        'PARAMS = {"a": 1}\n'
        "\n"
        "def build(params, out_path):\n"
        "    import json\n"
        '    with open(out_path, "w", encoding="utf-8") as fh:\n'
        f'        fh.write("{payload}")\n'
        '    with open(out_path + ".map.json", "w", encoding="utf-8") as fh:\n'
        '        json.dump({"k": "x" * 65536}, fh)\n'
        "\n"
        'if __name__ == "__main__":\n'
        "    import sys\n"
        "    build(PARAMS, sys.argv[1])\n"
    )


# ---------------------------------------------------------------------------
# REAL 构建脚本（per-target 常量；conftest profile 按 target 选用）
# ---------------------------------------------------------------------------

REAL_IFC_SCRIPT = '''\
import sys

import ifcopenshell

from script_lib import create_skeleton, write_and_validate

PARAMS = {"name": "sandbox-real", "storeys": {"1F": 0.0}}

def build(params, out_path):
    model = ifcopenshell.file(schema="IFC4")
    create_skeleton(model, name=params["name"], storeys=params["storeys"])
    write_and_validate(model, out_path)

if __name__ == "__main__":
    build(PARAMS, sys.argv[1])
'''

REAL_CAD_SCRIPT = '''\
import sys

import ezdxf

from cad_script_lib import add_entity, write_and_validate

PARAMS = {"length": 10}

def build(params, out_path):
    doc = ezdxf.new("R2010")
    msp = doc.modelspace()
    add_entity(msp, "LINE", start=(0, 0), end=(params["length"], 0))
    write_and_validate(doc, out_path)

if __name__ == "__main__":
    build(PARAMS, sys.argv[1])
'''
