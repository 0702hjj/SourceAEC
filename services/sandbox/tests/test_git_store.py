# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
import pytest
from aibim_sandbox import artifacts, git_store


def test_git_branches_merge_and_database_restore(tmp_path):
    root = str(tmp_path)
    artifacts.initialize(root)
    first = git_store.commit(root, "model", "a=1\nb=1\n", version="v1")
    with git_store.prepare(root, "model", "create", "feature") as (repo, branch, script):
        git_store.persist(root, "model", repo, branch)
    git_store.commit(root, "model", "a=2\nb=1\n")
    with git_store.prepare(root, "model", "checkout", "main") as (repo, branch, script):
        assert script == "a=1\nb=1\n"
        git_store.persist(root, "model", repo, branch)
    with git_store.prepare(root, "model", "merge", "feature") as (repo, branch, script):
        assert script == "a=2\nb=1\n"
        git_store.persist(root, "model", repo, branch)
    state = git_store.state(root, "model")
    assert state["branch"] == "main"
    assert set(state["branches"]) == {"main", "feature"}
    assert len(state["commits"][0]["parents"]) == 2
    assert first["commit"] in {c["commit"] for c in state["commits"]}


def test_conflict_keeps_saved_head(tmp_path):
    root = str(tmp_path)
    artifacts.initialize(root)
    git_store.commit(root, "model", "a=1\n")
    with git_store.prepare(root, "model", "create", "feature") as (repo, branch, _):
        git_store.persist(root, "model", repo, branch)
    git_store.commit(root, "model", "a=2\n")
    with git_store.prepare(root, "model", "checkout", "main") as (repo, branch, _):
        git_store.persist(root, "model", repo, branch)
    expected = git_store.commit(root, "model", "a=3\n")["commit"]
    with pytest.raises(ValueError):
        with git_store.prepare(root, "model", "merge", "feature"):
            pass
    assert git_store.state(root, "model")["commit"] == expected
