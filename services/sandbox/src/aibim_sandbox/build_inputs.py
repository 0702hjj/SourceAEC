# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Per-request input context; collection and projection are implemented in Go."""
from contextlib import contextmanager
from contextvars import ContextVar
from .go_runtime import call
_CURRENT = ContextVar("sourceaec_build_inputs", default=None)


def collect(root, model_id):
    return call("build.collect", root, {"model": model_id})


@contextmanager
def use(inputs):
    token = _CURRENT.set(inputs)
    try:
        yield
    finally:
        _CURRENT.reset(token)


def current():
    return _CURRENT.get() or {}


def fingerprint():
    return call("build.fingerprint", args={"inputs": current()})


def project(workdir):
    return call("build.project", args={"workdir": workdir, "inputs": current()})


def active():
    return _CURRENT.get() is not None
