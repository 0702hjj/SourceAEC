# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Python callable adapter for the Go task scheduler running inside the sandbox."""
from dataclasses import dataclass
import json
import os
from pathlib import Path
import subprocess
from typing import Callable


@dataclass(frozen=True)
class Task:
    key: str
    run: Callable[[dict, dict], object]
    params: tuple[str, ...] = ()
    dependencies: tuple[str, ...] = ()


def run_tasks(params: dict, tasks: list[Task]) -> dict:
    local = Path(__file__).with_name("sourceaec-runtime")
    binary = str(local) if local.is_file() else os.environ.get("SOURCEAEC_RUNTIME_BIN", "sourceaec-runtime")
    # No host credentials or database connectivity enter this process.
    env = {"PATH": os.environ.get("PATH", ""), "GOMAXPROCS": "2", "GOMEMLIMIT": "128MiB"}
    with subprocess.Popen([binary, "--tasks"], stdin=subprocess.PIPE,
                          stdout=subprocess.PIPE, text=True, env=env) as proc:
        def send(value):
            proc.stdin.write(json.dumps(value, ensure_ascii=False, allow_nan=False) + "\n")
            proc.stdin.flush()
        try:
            send({"params": params,
                  "tasks": [{"key": t.key, "params": t.params, "dependencies": t.dependencies} for t in tasks],
                  "incoming": os.environ.get("SOURCEAEC_TASK_INPUT", ".task-input"),
                  "outgoing": os.environ.get("SOURCEAEC_TASK_OUTPUT", ".task-output"),
                  "fingerprint": os.environ.get("SOURCEAEC_BUILD_FINGERPRINT", "uncached")})
            by_key = {t.key: t for t in tasks}
            for line in proc.stdout:
                message = json.loads(line)
                if message.get("error"):
                    raise ValueError(message["error"])
                if message["action"] == "done":
                    return message["result"]
                send({"result": by_key[message["key"]].run(message["params"], message["dependencies"])})
            raise RuntimeError("Go task scheduler exited without a result")
        finally:
            proc.stdin.close()
            if proc.poll() is None:
                proc.terminate()
