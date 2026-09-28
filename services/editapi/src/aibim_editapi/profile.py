# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""ServiceProfile —— ifc/cad 服务差异的显式声明类型（W-0057 T2）。

共享实现（aibim_editapi.config / script_runner）只认 profile 字段，不写
``if target == ...`` 式分叉；两侧 ``app/config.py`` 里的 PROFILE 声明就是
各自服务差异的**单点**（一眼可读）：

- 数据差异（env 名/默认值/产物名/前缀/默认依赖集）= 字段取值；
- 功能有无（drawlib 共享画法层、reset_state 内层 runner）= 空值/False
  显式声明，不存在隐式缺省分叉。
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Tuple


@dataclass(frozen=True)
class ServiceProfile:
    """一个 edit 服务的全部已知差异（取值见两侧 app/config.py 的声明）。"""

    # --- 身份与路径锚点 ---
    name: str  # 服务名（诊断/文档用；禁止用于逻辑分叉）
    ext: str  # 产物扩展名（ifc / dxf）
    service_root: Path  # 服务根目录（相对路径解析锚点 = app/ 上两级）

    # --- 端口与限额 env 名（ifc=EDIT_SERVICE_*，cad=CAD_SERVICE_*） ---
    port_env: str
    default_port: int
    max_models_env: str
    diff_timeout_env: str

    # --- flows 契约目录（ifc=skill 资产相对锚点；cad=服务内 flows） ---
    flows_dir_env: str
    default_flows_dir: str

    # --- drawlib 共享画法层（cad 独有；ifc 显式留空 → 恒不注入） ---
    drawlib_dir_env: str = ""  # env 名为空 = 不读 env
    drawlib_repo_rel_paths: Tuple[str, ...] = ()  # 相对 repo 根；空 = 无默认推导

    # --- script_runner 沙箱静态差异（→ sandbox_config() 折成 SandboxConfig） ---
    temp_prefix: str = "aibim-run-"
    product_name: str = "out.bin"
    product_label: str = "PRODUCT"
    flows_module: str = "script_lib"  # flows 契约校验器模块名
    validate_prefix: str = "aibim-validate-"
    default_deps: Tuple[str, ...] = ()  # T4 无 PEP 723 声明脚本的默认依赖集
    reset_state_inner_runner: bool = False  # True = inner runner 先 reset flows 模块状态

    # --- locate/edit-call 入参出参字段名（W-0057 T1；先例：契约套件 TargetProfile） ---
    # ifc：locate 查询 guid → designKey hop，edit-call body 用 designKey；
    # cad：locate 查询 key 即 XDATA key 本身，edit-call body 同名。共享路由按
    # 取值取名（query alias / 响应字段 / body 字段），差异=取值，零 if-target。
    locate_query_param: str = "key"
    locate_resp_field: str = "key"
    edit_body_field: str = "key"

    # --- 两侧一致的共同默认值（出现分叉时再提升为构造参数） ---
    default_max_models: int = 8
    default_diff_timeout_s: int = 60
