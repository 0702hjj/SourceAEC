# Geometry sandbox

Production execution requires Linux bubblewrap and util-linux prlimit; missing isolation fails closed.
Go creates a disposable work directory and read-only input/runtime mounts, with no host network or business credentials.
Interpreter symlink chains retain the named runtime roots and the resolved base without mounting a shared parent directory.

Default execution limits are 60 seconds, 1 GiB address space, 256 processes, 256 MiB files/products and 1 MiB cumulative logs.
Timeout/log overflow kills the process group; publication rejects symlinks and nonregular output files.
Environment preparation counts toward the execution slot and timeout. IFC/DXF share three process-local slots; overload returns 429.

Set `SOURCEAEC_PYTHON` to an absolute external interpreter, or let Go prepare pinned Python 3.12 environments with uv-managed CPython.
IFC defaults to pinned IfcOpenShell/numpy; DXF to ezdxf. PEP 723 dependencies must use exact registry versions;
local paths, URLs, unpinned versions and command-line options are rejected. Installation occurs before isolated script execution.

The model program receives its output path as the first argument. Optional source maps go to `SOURCEAEC_MAP_OUT`;
Go binds their source digest. Project inputs are captured from PostgreSQL and mounted read-only at `SOURCEAEC_INPUTS_DIR`
(and `VIEWER_DATA_DIR`), limited to the project's allowed plans/skill-work files. Versions archive these inputs for replay.

Durable workflows seal venv/base runtime contents and recheck before/after execution. A venv linked to system `/usr` Python is insufficient;
use an isolated managed base. This is drift detection, not an OS snapshot or determinism guarantee.
See [configuration](/en/operations/configuration) and [workflows](/en/reference/workflows).
