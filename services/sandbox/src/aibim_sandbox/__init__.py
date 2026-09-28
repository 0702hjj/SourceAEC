# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""aibim_sandbox — services/ifc 与 services/cad 的共享沙箱与 script-as-source 领域模块。

W-0048 T1：两侧逐行复制的实现合一到此包，服务侧只留薄适配（配置绑定）。

- ``spec``：运行常量 + SandboxConfig（服务间静态差异）+ RunSpec（单次执行规格）
- ``backend``：沙箱后端探测/fail-closed/rlimits/ro-binds/环境变量/bwrap 命令构造
- ``runner``：静态契约门、并发闸、沙箱执行泵循环、产物与 map 信封发布
- ``script_staging`` / ``script_params`` / ``script_edit`` / ``script_diff``：
  script-as-source 编辑面（原两侧 0-diff 复制件）
- ``script_versions`` / ``versions``：大版本快照（扩展名参数化）
- ``route_common``：跨服务请求解析 helper 单点（MODEL_ID_PATTERN/model_upload_path/
  model_lock + ModelLocks）
"""
