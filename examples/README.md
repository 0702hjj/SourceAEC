# AI_IFC Examples

IFC 时代的示例脚本。从仓库根运行（需 `services/ifc` 的 Python 环境，含 ifcopenshell）：

```bash
cd services/ifc && uv run python ../../examples/<script>.py
```

- `build_two_storey.py` — 用 ifcopenshell 直写一栋两层小楼（墙/板/开洞），演示骨架优先建模流程；产物 `two_storey.ifc` 可作为平台参考输入（web 上传入口已隐藏，`POST /api/v1/models` 仍可用，常规流程由 agent 在项目会话内建模）。
- `smoke_test_minimal.py` — 最小冒烟：建单墙模型并自检，用于快速验证 ifcopenshell 环境可用。
- `models/` — buildingSMART 官方样例 IFC（CC BY 4.0），可作为参考输入体验（或让 agent 经 bootstrap 复现为脚本模型），来源与许可证见 `models/README.md`。

本目录只保留当前 IFC/CAD 平台可运行的合成示例。
