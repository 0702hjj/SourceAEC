# Contributing to SourceAEC

Contributions are welcome through GitHub Issues and pull requests.

The project's origin and rights-handling principles are documented in
[PROJECT_PROVENANCE.md](PROJECT_PROVENANCE.md).

## Workflow

1. Open or reference an Issue for non-trivial changes.
2. Create a focused branch from `main`.
3. Add tests before implementation for features and bug fixes.
4. Run the affected component suite and `scripts/check_file_size.sh`.
5. Open a pull request and describe the behavior, verification, and any
   deployment impact.

## Contribution Provenance And Rights

Only submit material that has a lawful, documented provenance. By submitting
a contribution, you certify that:

- you created the contribution, or otherwise have the legal right to submit
  it under the repository's applicable license;
- any incorporated third-party material is identified in the pull request,
  including its source, copyright notice, and compatible license;
- the contribution does not contain confidential information, trade secrets,
  personal data, customer data, private drawings, or material obtained from a
  private repository without express authorization; and
- if an AI tool assisted the contribution, its inputs did not contain such
  restricted material and you reviewed the output for provenance and license
  compatibility.

Each commit must include a Developer Certificate of Origin sign-off. Use
`git commit -s` to add a `Signed-off-by` trailer. The sign-off certifies the
[Developer Certificate of Origin 1.1](https://developercertificate.org/).
Do not sign on behalf of another person. Maintainers may request source and
license information, reject a contribution, or remove material when its
provenance or authorization cannot be established.

Component setup and commands are documented at
<https://0702hjj.github.io/SourceAEC/>. Source changes belong in this repository;
documentation feedback can be filed as an Issue here.

## License

By contributing, you agree that your contribution is licensed under the
Apache License 2.0 unless a nested directory carries its own license.
