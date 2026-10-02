# Security Policy

Please do not disclose vulnerabilities or sensitive rights information in a
public Issue. If GitHub private vulnerability reporting is enabled for this
repository, use it; otherwise contact the maintainer privately through the
repository owner's GitHub profile. Include affected versions, reproduction
steps, impact, and any suggested mitigation, but omit secrets, personal data,
customer data, and confidential files.

The script execution service is designed to fail closed when its bubblewrap
sandbox is unavailable. Do not expose the Python edit services directly to
untrusted networks; use the Go gateway and configure authentication.

Treat user prompts, uploaded models, model output, and tool arguments as
untrusted. Run the Go server, agent tools, and skill CLIs under a dedicated
least-privilege operating-system account that has no unrelated credentials or
private files. The built-in bearer token is a single deployment credential;
it is not user-level authorization or multi-tenant isolation.

SourceAEC is not a safety-critical or compliance certification system. Do not
use generated models, AI output, conversion results, or sandbox behavior as a
substitute for qualified human review. Do not include secrets, personal data,
customer data, or confidential files in a vulnerability report.
