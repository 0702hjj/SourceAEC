# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Version-control integration contract, executed against both service profiles."""
from conftest import MODEL_ID, staging_script


def _save(client, script):
    assert client.put(f"/models/{MODEL_ID}/script", json={"script": script}).status_code == 200
    result = client.post(f"/models/{MODEL_ID}/script/save", json={"note": "checkpoint"})
    assert result.status_code == 200, result.text
    assert len(result.json()["commit"]) == 40
    return result.json()


def test_branch_checkout_merge_and_stale_revision(client, profile):
    _save(client, profile.key_script(profile.default_key))
    original = client.get(f"/models/{MODEL_ID}/script").json()
    body = {"branch": "feature", "baseRevision": original["revision"]}
    result = client.post(f"/models/{MODEL_ID}/git/branches", json=body)
    assert result.status_code == 200, result.text
    assert result.json()["branch"] == "feature"
    changed = profile.key_script(profile.default_key).replace('"wall_t": 0.2', '"wall_t": 0.4')
    # A textual note is sufficient to exercise a distinct source revision.
    changed += "\n# feature branch\n"
    _save(client, changed)
    current = client.get(f"/models/{MODEL_ID}/script").json()
    stale = client.post(f"/models/{MODEL_ID}/git/checkout", json={
        "branch": "main", "baseRevision": original["revision"]})
    assert stale.status_code == 409
    result = client.post(f"/models/{MODEL_ID}/git/checkout", json={
        "branch": "main", "baseRevision": current["revision"]})
    assert result.status_code == 200, result.text
    assert client.get(f"/models/{MODEL_ID}/script").json()["script"] == original["script"]
    result = client.post(f"/models/{MODEL_ID}/git/merge", json={
        "branch": "feature", "baseRevision": original["revision"]})
    assert result.status_code == 200, result.text
    assert client.get(f"/models/{MODEL_ID}/script").json()["script"] == changed


def test_run_then_save_reuses_successful_build(client, profile, data_dir, monkeypatch):
    from app import script_runner
    script = profile.key_script(profile.default_key)
    assert client.put(f"/models/{MODEL_ID}/script", json={"script": script}).status_code == 200
    assert client.post(f"/models/{MODEL_ID}/script/run").status_code == 200
    calls = []
    original = script_runner.run_script
    def count(*args, **kwargs):
        calls.append(1)
        return original(*args, **kwargs)
    monkeypatch.setattr(script_runner, "run_script", count)
    result = client.post(f"/models/{MODEL_ID}/script/save", json={})
    assert result.status_code == 200, result.text
    assert calls == []
    # The current product and its map survive a workspace loss.
    from aibim_sandbox import artifacts
    product = data_dir / "uploads" / f"{MODEL_ID}.{profile.ext}"
    expected = product.read_bytes()
    product.unlink()
    artifacts._ROOTS.discard(data_dir.resolve())
    artifacts.initialize(str(data_dir))
    assert product.read_bytes() == expected


def test_failed_candidate_preserves_branch_script_and_product(client, profile, data_dir):
    from aibim_sandbox import git_store
    _save(client, profile.key_script(profile.default_key))
    original = client.get(f"/models/{MODEL_ID}/script").json()
    product = data_dir / "uploads" / f"{MODEL_ID}.{profile.ext}"
    original_bytes = product.read_bytes()
    root = str(data_dir)
    with git_store.prepare(root, MODEL_ID, "create", "broken") as (repo, branch, _):
        git_store.persist(root, MODEL_ID, repo, branch)
    broken = original["script"].replace("def build(params, out_path):", "def build(params, out_path):\n    raise RuntimeError('candidate fails')")
    git_store.commit(root, MODEL_ID, broken)
    with git_store.prepare(root, MODEL_ID, "checkout", "main") as (repo, branch, _):
        git_store.persist(root, MODEL_ID, repo, branch)
    response = client.post(f"/models/{MODEL_ID}/git/checkout", json={
        "branch": "broken", "baseRevision": original["revision"]})
    assert response.status_code == 422
    assert git_store.state(root, MODEL_ID)["branch"] == "main"
    assert product.read_bytes() == original_bytes
    assert client.get(f"/models/{MODEL_ID}/script").json()["script"] == original["script"]
