# Project Origin, Authorship, and Rights Statement

This statement records the origin and development principles of SourceAEC. It
is intended to make the project's provenance transparent, reduce the risk of
confidential or improperly licensed material entering the repository, and
provide a clear process for raising rights concerns. It supplements, but does
not replace or modify, `LICENSE`, `NOTICE`, or the licenses of third-party
components.

## Independent Project

The maintainer states that SourceAEC was independently undertaken outside the
scope of the duties specified in the maintainer's internship agreement and was
not developed to perform an internship work assignment. The project did not
use the internship organization's material or technical resources. It was
independently implemented from public materials using personally funded
equipment, accounts, computing services, and other resources.

No private repository, source code, internal document, prompt, customer data,
drawing, model, trade secret, or other confidential material of the internship
organization was intentionally used as an implementation input or included in
this repository. Continued maintenance is likewise conducted as an independent
open-source activity.

These statements describe the maintainer's development records and good-faith
understanding of the facts. They are not a representation on behalf of any
current or former employer, client, or other third party.

## Public Technical Background

SourceAEC grew from public research and engineering exploration into
agent-friendly CAD and BIM interfaces. Its technical background includes the
public SimpleCADAPI project and the related research published in the journal
*Computer-Aided Design*, together with public buildingSMART IFC standards,
public documentation, open-source interfaces, and other publicly available
technical literature.

Those materials informed the problem framing and engineering direction.
SourceAEC is an independently implemented system for IFC authoring, editing,
inspection, and versioning, with optional planning and DXF workflows. A
reference to a public project or publication is attribution of background, not
a claim of affiliation, endorsement, or joint authorship. Third-party code,
assets, and data actually redistributed by this repository remain governed by
their respective licenses and notices.

## Development Record

The public Git history begins with an initial source snapshot and therefore is
not, by itself, a complete record of work performed before publication.
Subsequent changes are recorded through commits and pull requests. The
maintainer separately retains appropriate contemporaneous records of the
project's development and use of personal resources; private records are not
published merely to support this statement.

Contributors must provide a Developer Certificate of Origin sign-off and
follow the provenance requirements in `CONTRIBUTING.md`. Contributions with an
unclear origin, incompatible license, or unresolved confidentiality concern may
be rejected, quarantined, or removed.

## Licensing Boundaries

SourceAEC-authored material is offered under Apache-2.0 except where a nested
license or file-level SPDX identifier states otherwise. In particular,
`skills/aiplan/` and `skills/aidxf/` are MIT-licensed by default under their
local license files. This licensing decision does not claim ownership of, or
relicense, third-party material.

The IFC web experience contains two implementation paths:

- the xeokit path loads server-generated XKT and uses the AGPL-3.0
  `@xeokit/xeokit-sdk`; XKT conversion uses `@xeokit/xeokit-convert`; and
- the web-ifc path uses MPL-2.0 `web-ifc` and Three.js to read IFC directly in
  the browser without using XKT at runtime.

The paths are separate viewer implementations, but the repository's current
standard web build includes both paths and declares xeokit as a dependency.
Selecting the web-ifc path at runtime does not by itself remove xeokit from the
distributed build or eliminate obligations that may arise from distributing or
hosting that build. A deployment that intends to exclude xeokit must use a
separately verified build that does not include or depend on xeokit. See
`NOTICE` and the exact dependency versions in the lockfiles before distribution
or deployment.

## Rights Concerns

No license granted by this repository waives the maintainer's authorship or
other rights, and no statement here grants rights in third-party names,
trademarks, patents, confidential information, or separately licensed
materials.

Anyone who believes that a specific repository path infringes a copyright,
patent, contractual right, trade secret, or other lawful interest should
contact the maintainer privately through the repository owner's GitHub profile
or GitHub private vulnerability reporting when available. A report should
identify the affected path, the asserted right, the reporter's relationship to
that right, and enough non-confidential evidence to investigate the claim.
Sensitive evidence must not be posted in a public Issue.

The maintainer will review substantiated reports in good faith and may
temporarily restrict, replace, or remove disputed material while the issue is
investigated. This process is intended to resolve legitimate concerns without
discouraging lawful open-source use and contribution.
