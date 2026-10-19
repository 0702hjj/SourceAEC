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


def test_derived_build_metadata_does_not_conflict_with_source_merge(tmp_path):
    root = str(tmp_path)
    artifacts.initialize(root)
    source = "# first\na=1\n\n\n\n\n\n\n# last\nb=1\n"
    git_store.commit(root, "model", source, inputs={"current.build.json": {"productHash": "first"}})
    with git_store.prepare(root, "model", "create", "feature") as (repo, branch, _):
        git_store.persist(root, "model", repo, branch)
    git_store.commit(root, "model", source.replace("a=1", "a=2"),
                     inputs={"current.build.json": {"productHash": "feature"}})
    with git_store.prepare(root, "model", "checkout", "main") as (repo, branch, _):
        git_store.persist(root, "model", repo, branch)
    git_store.commit(root, "model", source.replace("b=1", "b=2"),
                     inputs={"current.build.json": {"productHash": "main"}})
    with git_store.prepare(root, "model", "merge", "feature") as (repo, branch, script):
        assert "a=2" in script and "b=2" in script
        git_store.persist(root, "model", repo, branch)
    assert len(git_store.state(root, "model")["commits"]) == 4
