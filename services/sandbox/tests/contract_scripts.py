# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""contract_scripts.py — 契约测试共用的构建脚本片段。

全部为两侧契约（aiifc script_lib / aidxf cad_script_lib 的
validate_script_contract）都接受的最小脚本：顶层 ``PARAMS`` 字面量 dict +
``build(params, out_path)`` + ``__main__`` 守卫。产物内容写纯文本占位
（不依赖 ifcopenshell/ezdxf），保证契约只钉沙箱行为、不钉格式库。
"""

from __future__ import annotations

GOOD_SCRIPT = '''\
PARAMS = {"name": "t"}

def build(params, out_path):
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write("PRODUCT /* " + params["name"] + " */")

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

# flows helper 在沙箱子进程内可 import（PYTHONPATH / ro-bind 挂载的契约）。
# 模块名由适配器声明（script_lib / cad_script_lib），{flows_module} 处替换。
FLOWS_IMPORT_SCRIPT = '''\
PARAMS = {{"a": 1}}

def build(params, out_path):
    import {flows_module}
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write("FLOWS-OK")

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

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

SYNTAX_ERROR_SCRIPT = "PARAMS = {"

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

NO_OUTPUT_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    open("elsewhere.out", "w").write("x")  # ignores argv[1]

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

STDOUT_FLOOD_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    import sys
    sys.stdout.write("F" * (4 << 20))
    sys.stdout.flush()
    open(out_path, "w").write("PRODUCT")

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

MODERATE_STDOUT_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    import sys
    sys.stdout.write("O" * 100000)
    sys.stdout.flush()
    open(out_path, "w").write("PRODUCT")

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

BIG_MAP_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    import json
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write("PRODUCT")
    with open(out_path + ".map.json", "w", encoding="utf-8") as fh:
        json.dump({"k": "x" * 65536}, fh)

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

MAP_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    import json
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write("PRODUCT")
    with open(out_path + ".map.json", "w", encoding="utf-8") as fh:
        json.dump({"0:wall:1": {"line": 7, "origin": "traced"}}, fh)

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

BAD_JSON_MAP_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write("PRODUCT")
    with open(out_path + ".map.json", "w", encoding="utf-8") as fh:
        fh.write("{not json")

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

NON_DICT_MAP_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    import json
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write("PRODUCT")
    with open(out_path + ".map.json", "w", encoding="utf-8") as fh:
        json.dump(["not", "a", "dict"], fh)

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

# {target} 处替换为沙箱外路径的字面量（bwrap 下写入必须失败）。
ESCAPE_WRITE_SCRIPT = '''\
PARAMS = {{"target": {target!r}}}

def build(params, out_path):
    open(params["target"], "w").write("pwned")
    open(out_path, "w").write("PRODUCT")

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''

NET_PROBE_SCRIPT = '''\
PARAMS = {"a": 1}

def build(params, out_path):
    import socket
    try:
        socket.create_connection(("8.8.8.8", 53), timeout=3)
        result = "NET-OPEN"
    except OSError:
        result = "NET-BLOCKED"
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write(result)

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

# fork 数量占位 {forks}（MAX_PROCS + 余量，超 RLIMIT_NPROC 预算）。
FORK_BOMB_SCRIPT = '''\
PARAMS = {{"a": 1}}

def build(params, out_path):
    import os, time
    for _ in range({forks}):
        if os.fork() == 0:
            time.sleep(5)
            os._exit(0)
    with open(out_path, "w", encoding="utf-8") as fh:
        fh.write("FORKED")

if __name__ == "__main__":
    import sys
    build(PARAMS, sys.argv[1])
'''
