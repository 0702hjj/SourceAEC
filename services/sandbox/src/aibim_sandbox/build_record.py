# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Runtime facts passed to Go for build identity, reuse and publication."""
from pathlib import Path
import sys
from . import artifacts, deps, build_inputs
from .go_runtime import call


def arguments(spec, script, env):
    return {"script": script, "envDir": str(env.env_dir), "python": sys.version.split()[0],
            "helpers": [str(spec.flows_dir), *map(str, spec.extra_pythonpath)],
            "engine": str(Path(__file__).parent), "productName": spec.config.product_name,
            "dependencies": list(env.deps), "inputs": build_inputs.current()}


def identity(spec, script, env):
    return call("build.identity", args=arguments(spec, script, env))


def reusable(spec, script, product, map_path):
    if not Path(product + ".build.json").is_file():
        return False
    env = deps.ensure_env(spec, script)
    return call("build.reusable", args={**arguments(spec, script, env), "product": product, "map": map_path})


def publish(spec, script, product, map_path, env, task_stats=None):
    match = artifacts.owner(product)
    call("build.publish", str(match[0]) if match else "", {
        **arguments(spec, script, env), "product": product, "map": map_path, "stats": task_stats})
