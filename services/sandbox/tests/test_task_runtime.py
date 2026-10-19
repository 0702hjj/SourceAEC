# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
import shutil
import pytest
from aibim_sandbox.task_runtime import Task, run_tasks


def test_only_affected_task_and_dependents_rerun(tmp_path, monkeypatch):
    incoming, outgoing = tmp_path / "in", tmp_path / "out"
    monkeypatch.setenv("SOURCEAEC_TASK_INPUT", str(incoming))
    monkeypatch.setenv("SOURCEAEC_TASK_OUTPUT", str(outgoing))
    monkeypatch.setenv("SOURCEAEC_BUILD_FINGERPRINT", "code-and-environment")
    calls = []
    def task(key, parameters=(), dependencies=()):
        def run(params, deps):
            calls.append(key)
            return {"params": params, "dependencies": deps}
        return Task(key, run, parameters, dependencies)
    tasks = [task("wall", ("height",)), task("window", (), ("wall",)), task("slab", ("width",))]
    baseline = run_tasks({"height": 3, "width": 10}, tasks)
    shutil.copytree(outgoing, incoming)
    calls.clear()
    updated = run_tasks({"height": 4, "width": 10}, tasks)
    assert calls == ["wall", "window"]
    assert updated["slab"] == baseline["slab"]
    shutil.rmtree(incoming)
    calls.clear()
    assert run_tasks({"height": 4, "width": 10}, tasks) == updated
    assert calls == ["wall", "window", "slab"]


def test_cycle_and_duplicate_keys_fail(tmp_path, monkeypatch):
    monkeypatch.setenv("SOURCEAEC_TASK_OUTPUT", str(tmp_path))
    task = Task("a", lambda p, d: 1, dependencies=("a",))
    with pytest.raises(ValueError, match="cyclic"):
        run_tasks({}, [task])
    with pytest.raises(ValueError, match="duplicate"):
        run_tasks({}, [task, task])
