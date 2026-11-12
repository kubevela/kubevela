# Dependency Management Policy

This document describes how the KubeVela project manages third-party
dependencies in this repository.

## Declaring dependencies

- Go dependencies are declared in `go.mod` with checksums recorded in
  `go.sum`, which the Go toolchain verifies on every build.
- New dependencies are introduced through regular pull requests and go
  through the same review as any other change, by the reviewers, approvers
  and maintainers listed in the
  [community OWNERS file](https://github.com/kubevela/community/blob/main/OWNERS.md).
- Dependency licenses must be approved: CI runs `license_finder` against the
  decisions recorded in `.license/dependency_decisions.yml` and fails the
  build when an unapproved license is introduced.

## Keeping dependencies up to date

- [Dependabot](../.github/dependabot.yml) opens weekly update pull requests
  for Go modules and for GitHub Actions.
- `k8s.io/*` modules are excluded from automated bumps. Kubernetes libraries
  are tightly coupled to the supported Kubernetes versions, so they are
  upgraded deliberately as part of coordinated compatibility upgrades.
- Dependency update pull requests run the same CI (unit, e2e and lint jobs)
  as any other change before they can be merged.

## Handling vulnerable dependencies

- Vulnerabilities can be reported privately following the process in
  [SECURITY.md](../SECURITY.md).
- Automated detection runs continuously: Trivy scans the vela-core image on
  pull requests and uploads results to the GitHub Security tab, CodeQL
  analyzes pushes to master and release branches, and OpenSSF Scorecard runs
  weekly.
- Fixes land on master first. Supported release branches receive fixes
  through the repository's backport workflow when they are affected.

## Supply chain hygiene

- GitHub Actions used in CI workflows are pinned to commit SHAs.
- Release artifacts ship with SPDX SBOMs generated with syft during the
  GoReleaser release process.
