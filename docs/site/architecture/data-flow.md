# 数据流与事务

下图区分输入快照、外部执行、原子发布和派生转换。节点 ID 与[架构总览](/architecture/overview)一致。

窄屏上可横向滚动图表，避免将整张架构图缩小到难以阅读。

```mermaid
sequenceDiagram
    participant CLIENT as Web / Agent / MCP
    participant API as Go 网关
    participant CAPTURE as 固定输入捕获
    participant SANDBOX as 外部几何沙箱
    participant PUBLISH as Go 发布
    participant PG as PostgreSQL
    participant QUEUE as IFC 转换
    CLIENT->>API: 暂存源码与 baseRevision
    API->>PG: revision 校验 / 提交暂存
    alt 显式持久 run/save
        CLIENT->>API: key / action / baseRevision
        API->>PG: 捕获前预留容量
        API->>CAPTURE: 固定源码、输入、环境摘要
        CAPTURE->>PG: 定义、blob、run 同事务提交
        API-->>CLIENT: 202 / workflow ID
        PG->>CAPTURE: worker 领取 / lease 与 attempt
    else 同步 run/save
        CLIENT->>API: run 或 save
        API->>CAPTURE: 短只读快照
    end
    CAPTURE->>SANDBOX: 关闭快照后传入固定输入
    Note over SANDBOX,PG: 环境准备与几何执行不持 SQL 连接或模型锁
    SANDBOX-->>PUBLISH: 完整产物与源映射
    PUBLISH->>PUBLISH: 检查格式、映射与结构预算
    opt 持久几何完成
        PUBLISH->>PG: 候选 blob + geometry checkpoint 同事务
    end
    PUBLISH->>PG: 共同准入 / 模型锁 / 重查身份和 fence
    PUBLISH->>PG: 产物、save 版本、publication 标记同事务
    Note over PUBLISH,PG: Journal 保护文件投影；提交凭据处理 COMMIT 应答丢失
    opt IFC
        PUBLISH->>QUEUE: 已提交源与 build 身份
        QUEUE->>QUEUE: 私有目录内 Node 转换
        QUEUE->>PG: 重查身份并发布完整预览 generation
    end
    CLIENT->>API: 同步结果 / 轮询工作流 / 读取预览
```

## 三种完成点

- `run` 发布当前产物，不创建版本；`save` 执行并保存不可变版本。
- 持久 API 的 202 表示固定定义已接受，不代表几何、版本或预览已完成。
- IFC 模型提交与预览转换分别完成。转换恢复不会再次保存版本；DXF 由 Go 直接生成 render 数据。

## 读取路径

```mermaid
flowchart LR
    PG[(已提交 PostgreSQL 快照)] --> EXPORT[按同一快照大小预留导出源字节]
    EXPORT --> SNAP[私有文件 / 打开后 unlink]
    SNAP --> RELEASE[归还导出预算与数据库连接]
    RELEASE --> CLIENT[Range / 慢客户端]
    MANIFEST[匹配源身份的预览清单] --> PREVIEW[锁内打开不可变文件句柄]
    PREVIEW --> CLIENT
```

导出源字节预算在导出结束归还；快照文件名额随客户端读取释放。
这些额度不是 PG 内存、临时空间或整个系统 RSS 的硬上限。
详见[预览与读取](/architecture/previews)和[资源验证](/operations/performance)。
