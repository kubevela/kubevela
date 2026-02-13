# Repository Guidelines

## Project Structure & Module Organization
KubeVela code lives under `pkg/` for controllers and reusable packages, while CLI entry points sit in `cmd/core/main.go` for the manager and `cmd/plugin/main.go` for kubectl glue. API types and generated clients belong in `apis/`, Helm assets land in `charts/`, and CRDs ship under `charts/vela-core/crds`. Place deterministic fixtures in `testdata/`, unit suites beside implementations as `*_test.go`, and end-to-end flows under `test/` or `e2e/`.

## Build, Test, and Development Commands
Use `make build` to compile CLI binaries into `bin/`. Run `make manager` to build the controller manager and `make run` (`make core-run`) to launch it against your current kubeconfig. Execute `make test` for envtest-backed unit suites, then narrow down with `go test ./pkg/<package> -v`. Finish local validation with `make lint`, `make staticcheck`, `make reviewable`, and `make check-diff`.

## Coding Style & Naming Conventions
Format every touched Go file via `go fmt` and `goimports` (module `github.com/oam-dev/kubevela`). Prefer concise, lowercase package names; export identifiers only when they cross packages. Keep comments focused on non-obvious behavior, and align file names with their primary type or feature.

## Testing Guidelines
Author table-driven Go tests next to the code they target (`<name>_test.go`). Maintain deterministic integration specs in `test/` or `e2e/`, and ensure `make test` produces an up-to-date `coverage.txt`. When debugging, run `go test -run <Case>` within the relevant package.

## Commit & Pull Request Guidelines
Follow Conventional Commits (e.g., `Feat(cli): improve init wizard`) and reference GitHub issues when available. PR descriptions should outline context, expected behavior, validation steps, and residual risks. Before pushing, capture the outputs of `make reviewable`, `make check-diff`, `make test`, and `make lint`; include screenshots for user-facing updates.

## Security & Configuration Tips
Apply or remove CRDs using `make core-install` and `make core-uninstall`. Never commit kubeconfigs, credentials, or unvetted binaries; prefer environment variables or secret stores for sensitive data.
