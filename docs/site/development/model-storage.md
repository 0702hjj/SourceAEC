# 模型存储与增量构建

平台以 Python 构建脚本为模型编辑事实源，使用 SQLite 保存业务文件和
Git 仓库 bundle。Go 网关与 IFC/CAD 服务必须使用相同的绝对
`VIEWER_DATA_DIR`，数据库位于该目录的 `sourceaec.sqlite3`。
SQLite 是本机部署的默认持久化方案；已有可选 PostgreSQL 业务表保持兼容。
Go SQLite 驱动需要 CGO 和 C 编译器；服务器还需要 Git 可执行文件。

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

配置 PostgreSQL 时也须单独备份其业务数据库。

SQLite 使用 WAL。在线备份应使用 SQLite backup API；停止全部写入进程后
也可以备份数据库文件及尚未 checkpoint 的 WAL，不能只复制活动数据库主文件。
可用 Python 标准库执行在线备份：

```python
import sqlite3
with sqlite3.connect("data/sourceaec.sqlite3") as source:
    with sqlite3.connect("sourceaec-backup.sqlite3") as destination:
        source.backup(destination)
```

恢复时停止所有服务，把备份恢复为数据库文件，再启动服务重建工作区。
启动时会为缺失 XKT 的已就绪模型重新排队转换；CAD render.json 可由服务重建。

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
