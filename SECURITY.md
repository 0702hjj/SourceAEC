# Security Policy

Please do not disclose vulnerabilities in a public Issue. Use GitHub's
private vulnerability reporting for this repository. Include affected
versions, reproduction steps, impact, and any suggested mitigation.

The script execution service is designed to fail closed when its bubblewrap
sandbox is unavailable. Do not expose the Python edit services directly to
untrusted networks; use the Go gateway and configure authentication.
