# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Version-control endpoints. Branch checkout/merge publish only after a build."""
from fastapi import APIRouter, HTTPException, Path, Request
from pydantic import BaseModel

from aibim_sandbox import git_store, build_inputs, artifacts, deps as sandbox_deps
from aibim_sandbox.route_common import MODEL_ID_PATTERN, model_lock, model_upload_path
from aibim_sandbox.go_runtime import call
from .routes_script_run import make_run_into_uploads
from .routes_script_staging import staging_of, verify_current_script


class BranchBody(BaseModel):
    branch: str
    baseRevision: str


def verify_clean(staging, revision: str) -> None:
    current = verify_current_script(staging, 409)
    verify_git_operation(lambda: call("edit.clean", args={
        "script": current, "revision": revision, "staged": staging.staged_count()}))


def verify_git_operation(operation):
    try:
        return operation()
    except (ValueError, OSError) as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc


def build_git_router(profile, deps) -> APIRouter:
    router = APIRouter()
    run = make_run_into_uploads(deps, profile.ext)

    @router.get("/models/{id}/git")
    def state(request: Request, id: str = Path(pattern=MODEL_ID_PATTERN)):
        model_upload_path(request, id, profile.ext)
        with model_lock(request, id, profile.ext):
            root = request.app.state.settings.data_dir
            verify_git_operation(lambda: git_store.import_history(root, id))
            return verify_git_operation(lambda: git_store.state(root, id))

    def change(request, id, body, action):
        root = request.app.state.settings.data_dir
        model_upload_path(request, id, profile.ext)
        with model_lock(request, id, profile.ext):
            staging = staging_of(request, id)
            verify_clean(staging, body.baseRevision)
            def apply():
                try:
                    with artifacts.transaction(root):
                        with git_store.prepare(root, id, action, body.branch) as (repo, branch, script):
                            metadata = git_store.candidate_inputs(root, id, repo)
                            with build_inputs.use(metadata["inputs"]), sandbox_deps.use_dependencies(
                                tuple(metadata["requirements"]) or None, metadata["environment"]
                            ):
                                run(request, id, script)
                            git_store.persist(root, id, repo, branch)
                            request.app.state.script_staging.reset(id, base=script)
                except BaseException:
                    request.app.state.script_staging.restore(staging)
                    raise
                return {"modelId": id, **git_store.state(root, id)}
            return verify_git_operation(apply)

    @router.post("/models/{id}/git/branches")
    def create(request: Request, body: BranchBody, id: str = Path(pattern=MODEL_ID_PATTERN)):
        return change(request, id, body, "create")

    @router.post("/models/{id}/git/checkout")
    def checkout(request: Request, body: BranchBody, id: str = Path(pattern=MODEL_ID_PATTERN)):
        return change(request, id, body, "checkout")

    @router.post("/models/{id}/git/merge")
    def merge(request: Request, body: BranchBody, id: str = Path(pattern=MODEL_ID_PATTERN)):
        return change(request, id, body, "merge")

    return router
