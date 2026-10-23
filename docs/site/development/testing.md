# 测试与边界检查

Go 测试位于实现旁。编辑领域测试覆盖暂存与冲突、版本/Git、失败不发布、
格式 diff、DXF render 以及真实 bubblewrap 执行和资源限制。
Python 只作为部分集成测试使用的外部几何运行时；测试本身用 Go 编写。

```bash
cd server
go test ./...
go vet ./...
```

设置独立测试数据库的 `VIEWER_PG_DSN`、`VIEWER_TEST_PG_DSN`。
几何库端到端测试可设置 `SOURCEAEC_TEST_GEOMETRY=1`，并通过
`SOURCEAEC_PYTHON` 提供安装 IfcOpenShell 与 ezdxf 的环境。
执行测试需要 bubblewrap 和 prlimit；无相应运行时的测试会说明跳过原因。

其他组件：`web` 执行 npm test/lint/build；`converter` 执行 npm test；
`docs` 执行 npm run docs:build。

根目录执行 `scripts/check_file_size.sh`、`scripts/check_public_snapshot.sh` 和
`scripts/check_go_business.sh`。业务边界检查禁止提交 Python 源码或应用清单，
skill 只维护声明和参考，不捆绑执行库。
