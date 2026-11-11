# Verification and tests

Tests live beside Go implementations. Editing tests cover staging/conflicts, versions/Git, failure without publication,
format comparisons, DXF render data and real bubblewrap isolation. Python is only an external geometry runtime, not the test implementation.

```bash
cd server
go test ./...
go vet ./...
```

Set VIEWER_PG_DSN and VIEWER_TEST_PG_DSN to a dedicated test database. Real geometry tests use SOURCEAEC_TEST_GEOMETRY=1
and SOURCEAEC_PYTHON with IfcOpenShell/ezdxf. Individual tests explain missing runtime skips.

The strict root entry `scripts/test_go_integration.sh` requires an explicit dedicated VIEWER_TEST_PG_DSN plus Go, Git, bubblewrap,
prlimit, uv, Node and converter dependencies. It enables real IFC/DXF/converter tests and five focused race repetitions.
It covers committed input snapshots, stale results, preview generations, process termination, cancellation, deletion,
publication rollback/lost COMMIT acknowledgements, REST/MCP authentication and phase recovery.

Key entry points include `editing/workflow_recovery_test.go`, `editing/workflow_conversion_test.go`,
`editruntime/workflow_receipt_test.go`, `api/native_editing_test.go`, `api/native_mcp_test.go` and
`editing/runtime_alias_test.go`. The latter checks interpreter aliases and private sibling isolation.

Run web tests/lint/build, converter tests and docs tests/build for their changes. Before a PR run
`scripts/check_file_size.sh`, `scripts/check_public_snapshot.sh`, `scripts/check_go_business.sh`.
No maintained Python business source or bundled SDK is permitted. Use [performance verification](/en/operations/performance)
separately from correctness/fault tests.

For the optional external SDK adapter, use `scripts/test_notebook_integration.sh` with VIEWER_TEST_PG_DSN and SOURCEAEC_TEST_NOTEBOOK_PYTHON. A separate CI job installs published SimpleCADAPI 2.2.0b1 and checks real cold/hot/partial equivalence, private concurrent copies, frozen budgets, persistence and cleanup.
