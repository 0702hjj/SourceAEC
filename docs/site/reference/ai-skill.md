# Agent Skills

SourceAEC 以 Skill 包提供面向 Agent 的建模能力。Agent 加载后可以编写代码或运行受支持的命令；Skill 与 [Agent 接入](/reference/ai) 的 REST 方式互补：REST 适合对既有脚本做定向操作，Skill 适合从零建模和大幅修改。Skill 与平台解耦，不部署整套平台也能单独使用。

本文使用 **IFC Authoring Skill**、**Plan Preparation Skill** 和 **DXF Authoring Skill** 作为公开能力名称。`aiifc`、`aiplan`、`aidxf` 与 `aidxfv3` 是当前目录和 CLI 的技术标识，示例中保持原样。

## 管线总览

IFC Authoring Skill 可以独立从设计输入构建 IFC。需要二维图纸作为上游时，可选用 `aiplan` 把外部资料整理成任务书，再由 `aidxf` 生成 DXF，最后交给 `aiifc` 消化。

```
外部资料 ──► aiplan ──┬─► plan.json（任务书）──────────► aidxf v3 ──► building.json + 各层 DXF ──┬─► bim（ifc）
                      └─► bim_supplement.json（BIM 补充）──────────────────────────────────────┘    ▲
                                                                                                   │
                             ifc 独立管线：design.json（LLM 草稿）──► aiifc ──────────────────────────┘
                             cad→ifc 消化管线：building.json + 各 zone DXF ──► aiifc consume-upstream
                                 ──► design.json ──► design-build（features.json）──► build-script（IFC）
```

| skill | 阶段 | 输入 | 输出 |
| --- | --- | --- | --- |
| `aiifc` | IFC 核心 | 独立使用 design 草稿，或消费 building、bim_supplement、DXF | 构建脚本加 IFC |
| `aiplan` | 可选规划入口 | 任意外部资料：图片、PPT、文档、对话 | `plan.json` 加 `bim_supplement.json` |
| `aidxfv3` | 可选 DXF 阶段 | `plan.json` 只读，加用户补充描述 | `building.json` 加各层 DXF |

## IFC Authoring Skill（`aiifc`）

面向 Agent 的 IfcOpenShell 建模 Skill，让 Agent 直接编写 `ifcopenshell.api` 代码。

skill 结构是 SKILL.md 加参考资产：SKILL.md 是行为宪法；references 里有 103 个 API 分页、8 个组件 recipe 和 13 个可运行 flows；templates 是可复制的完整脚本示例。

agent 建模的顺序是骨架先行：先 Project 到 Storey 的空间结构，再墙板梁柱等构件，然后开洞和门窗，再补类型材质属性集，最后导出并校验。复杂户型可以先出 design JSON 草稿辅助构思，经规范化后再生成脚本。

aiifc 提供五个 CLI，都支持 `--project-id` 把中间产物规范落到项目工作区：

| 命令 | 输入 | 输出 |
| --- | --- | --- |
| `aiifc consume-upstream` | building.json、bim_supplement.json、DXF 目录 | design.json，上游几何到楼层的精确映射 |
| `aiifc design-build` | design.json | features.json |
| `aiifc build-script` | features.json | 演示 IFC |
| `aiifc design-review` | 产物 IFC | 质量审查报告 |
| `aiifc ifc-inspect` | 产物 IFC | 结构检查 JSON |

cad 到 ifc 的消化路径：agent 先把上游产物桥接进工作区，跑 consume-upstream 和 design-build，然后在已有脚本上深化，是增量修改不是重写。

## Plan Preparation Skill（`aiplan`）

管线入口。把外部资料归一成下游可执行的任务书，全程用提问工具和用户确认设计意图。它不画图、不写 IFC、不做坐标级布局。

输出两个文件，schema 事实源在包内 `references/schemas/`：plan.json 给 cad 用，说明要什么在哪盖按什么规范；bim_supplement.json 给 bim 用，补 CAD 覆盖不了的屋顶、特殊结构、属性集。成对产出，过门禁校验后落盘。仅依赖 jsonschema，自包含可迁移。

## DXF Authoring Skill（`aidxf`）

可选的 DXF 制图框架，后续相关迭代都在这上面。

输入是 plan.json，全程只读，加用户的补充描述。输出是 building.json 和逐层 DXF。

分工原则是 LLM 设计、机器锚定：LLM 只声明哪里有分区、房间多大、和谁相邻，坐标全部交给机器派生，`aidxfv3 normalize` 是唯一的坐标计算点。流程分五步，每步有断点确认，多楼层裙房塔楼等场景按 zone 并行，支持中断恢复。依赖 ezdxf 和 shapely。

## 获取与安装

skill 是 agent 无关的目录包，opencode、Claude Code、Cursor 等都能加载。

下载：从 GitHub Release 取 `<name>-<version>.tar.gz`，或在仓库内自助打包：

```bash
python tools/skill_pack.py --skill aiifc --archive                 # aiifc
python tools/skill_pack.py --skill aidxf --archive                 # aidxf
python tools/skill_pack.py --skill-dir skills/aiplan --archive     # aiplan 走 --skill-dir
```

产物在 `skills/dist/`。解压到 agent 的 skill 目录即完成安装，运行时会自动索引 SKILL.md：

- 支持 skill 的 agent：安装到该 agent 的用户级或项目级 skill 目录
- Claude Code：`~/.claude/skills/<name>/`

运行依赖见各包内 requirements.txt：aiifc 要 ifcopenshell、ifcquery、numpy；aidxf 要 ezdxf 和 shapely；aiplan 只要 jsonschema。

与平台的关系：Skill 是 Agent 侧入口，编辑服务是服务端运行时，两者配对但可独立使用。只做一次性生成不需要整套平台；需要版本、diff 以及 Agent 与人协作编辑时才需要部署平台。

## 与 REST API 的关系

| 方式 | 场景 | 入口 |
| --- | --- | --- |
| REST 编辑 API | 在既有脚本上定向修改，版本与 diff | `:8100/models/{id}/...`，见 [Agent 接入](/reference/ai) |
| IFC Authoring Skill（`aiifc`） | 从零建模型、大改几何、复现上传 IFC | Agent 直接写 Python |
| Plan/DXF Skills（`aiplan`、`aidxf`） | 可选的 plan 到 DXF 工作流 | Agent 运行 CLI 命令 |

## 版本化发布

每个 skill 独立版本化，版本号写在 SKILL.md frontmatter，与平台版本解耦。发布四步：改版本号并在包内 CHANGELOG 追加条目；用打包器产出 tar.gz；跑 `python -m pytest tests/skill/ -q` 校验；打 tag 并建 GitHub Release。

## 许可

aiifc 是 Apache-2.0，文档参考自 IfcOpenShell 官方文档；aiplan 和 aidxf 目录默认采用 MIT，单个文件的 SPDX 标识优先于目录默认许可。
