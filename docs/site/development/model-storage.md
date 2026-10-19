# 模型存储与增量构建

平台以 Python 构建脚本为模型编辑事实源，业务持久化统一使用 PostgreSQL。
Go 实现业务文件存储、事务、Git bundle、构建身份、任务调度与缓存校验；
Python 负责 IfcOpenShell/ezdxf 调用、Python AST 分析和脚本函数执行。
Python 适配层通过私有 stdin/stdout JSON 协议调用 `sourceaec-runtime`，
不直接访问数据库或启动 Git。沙箱内的 Go 调度器使用独立 `--tasks` 模式，
不接收数据库凭据；生产隔离仍强制使用 bubblewrap。

三个服务设置相同的 `VIEWER_PG_DSN`、绝对 `VIEWER_DATA_DIR` 和
`VIEWER_STORAGE_NAMESPACE`。后者决定业务文件与 Git 的 PostgreSQL schema，
建议显式设置，移动工作区时保持不变。未设置时按绝对数据目录派生。
IFC/CAD 服务另需 `SOURCEAEC_RUNTIME_BIN` 指向 Go 运行时的绝对路径，
可在仓库根执行 `scripts/build_runtime.sh` 构建；需要 Git 可执行文件。
数据库不可用时启动失败，不回退到文件存储。

## 持久数据与工作区

持久数据包含模型、项目、方案、审查记录、聊天历史、脚本暂存与版本、
构件定位映射、构建输入及当前成功生成的 IFC/DXF。Git 提交包含脚本和
构建输入/环境清单，完整 bundle 存入数据库。

现有路径继续作为工具工作区：沙箱、转换器、Git 和下载端点仍可以使用
文件名。业务写入通过数据库适配层持久化；启动时恢复丢失或过期的投影。
不要在服务运行时手动修改或删除工作区。

首次启动自动导入已有业务文件，数据库内的迁移标记防止重复导入。
迁移前停止三个服务并备份原数据目录。首次迁移完成并确认可恢复之前，
保留原备份。目录中的历史派生产物不是数据库的持久存储对象。

XKT、metadata、render.json、任务缓存和历史 IFC/DXF 是可重建数据。
保存检查点后只保留最新版本的产品快照；历史脚本长期保存，旧产物按需
重建并进入有限缓存。上传原件 bootstrap 用于复现与对齐，独立保留。

## SQLite 升级与备份

旧版本 SQLite 数据库必须显式离线迁移，不能只导入工作区文件，后者不包含
完整 Git bundle。停止全部服务，备份原数据目录及 SQLite 数据库与 WAL，
准备空的 PostgreSQL 命名空间，再从 `server/` 执行：

```bash
export VIEWER_PG_DSN='postgres://localhost/sourceaec'
export VIEWER_STORAGE_NAMESPACE=sourceaec
# data-dir 必须与后续服务指向同一工作区
go run ./cmd/migrate-sqlite --source ../data/sourceaec.sqlite3 --data-dir ../data
```

迁移工具只读打开 SQLite，将业务文件、模式位和 Git bundle 一起提交到
PostgreSQL。目标已有数据或已初始化时拒绝覆盖；失败整批回滚。
仅迁移工具需要 CGO 与 C 编译器，网关和业务运行时不依赖 SQLite 驱动。
存在旧 SQLite 且尚未迁移时，服务会拒绝启动，避免静默丢失 Git 历史。
迁移成功后启动服务恢复工作区。原有 JSON 审查记录会导入已有的 PostgreSQL
审查表，保留 ID 和时间；导入标记防止删除记录在重启时复活。

日常备份使用 `pg_dump` 备份完整业务数据库，包含 schema 内的 artifacts、
repositories、迁移标记，以及审查表。恢复到 PostgreSQL 后，以相同命名空间
启动服务即可恢复工作区；转换器与沙箱缓存可重建。辅助库源码和依赖环境
仍需随部署版本保存。首次验证完成前保留旧备份。

## Git 版本控制

公开接口都经 Go 网关 `/api/v1` 暴露，响应遵循统一 envelope：

| 操作 | 端点 |
| --- | --- |
| 查看分支和提交 | `GET /models/{id}/git` |
| 创建并切换分支 | `POST /models/{id}/git/branches` |
| 切换分支 | `POST /models/{id}/git/checkout` |
| 合并到当前分支 | `POST /models/{id}/git/merge` |

写请求使用 `{"branch":"feature","baseRevision":"当前脚本 SHA-256"}`。
必须先保存或放弃暂存。合并使用真实 Git 三方合并；冲突返回 409，
保留原分支。候选脚本构建失败时不发布新产物，也不切换仓库状态。
已有脚本检查点在首次访问 Git 或再次保存时导入为提交和版本标签。

每个模型保留一份当前成功产物，各分支预览通过切换后构建得到。
历史回退恢复输入与精确 Python 包版本要求，依赖包仍需在缓存或包源中可用。
辅助库指纹用于缓存失效；辅助库源码本身尚未按历史版本归档，部署升级前
需要保留对应代码版本，不能把任意跨部署重建视为完全可复现。

## 前端编辑

IFC 选中构件后可以编辑映射到脚本调用的标量参数。直接引用 PARAMS 的参数
默认修改共享值，也可选择只覆盖此调用。脚本 revision 变化或映射过期时
拒绝写入。动态表达式、歧义调用点和无法映射的构件继续通过脚本编辑器修改。
`editable_elements.create_wall` 提供尺寸、位置与名称都可映射的墙体工厂。

修改成功生成预览，显式保存才创建检查点与 Git 提交。试跑后保存会在
脚本、环境、输入和当前产物都匹配时复用成功结果，避免重复沙箱执行。

## 增量执行边界

普通 Python 脚本仍完整执行。选择采用 `sourceaec_tasks` 的脚本可把计算拆成
有稳定 key 的任务：每项任务声明参数和依赖，只接收声明的输入，返回
JSON 兼容结果。可缓存独立 IFC 片段字符串，也可缓存 DXF 几何配方。
任务必须是确定性的，不能读取未声明的状态、当前时间或随机数。

修改参数后，受影响任务重新执行。依赖任务的输出未变时，下游结果可以复用。
脚本代码、辅助库、构建输入或环境变化时保守失效；缓存损坏或缺失时重算。
最终完整 IFC/DXF 的装配、序列化与校验每次仍执行。
合成示例是仓库的 `examples/incremental_walls.py`，可直接通过 IFC 服务暂存试跑。

web-ifc 查看器保留场景和相机，以 GlobalId 分组比较几何、变换和颜色，
保留未变化的 GPU 网格。当前仍下载和解析完整 IFC；默认 xeokit 仍完整
转换和加载 XKT。SSE 推送 Agent 过程事件，模型状态和 revision 通过 REST
获取；不能将这些能力描述成完整的增量 IFC 网络传输。
