# Declarative skill integration

The official Eino skill middleware reads `skills/*/SKILL.md`. Role filtering and
filesystem access remain implemented in Go. Skills provide necessary interface
and domain guidance without bundling a runtime SDK or Python CLI.

Read tools are confined to configured roots. Host command execution is disabled
by default; generated geometry source runs only through the Go sandbox executor.
Use the editing API for business changes and preserve optimistic revisions.
