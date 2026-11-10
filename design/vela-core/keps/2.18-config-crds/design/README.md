# KEP-2.18 Design Notes (companion design)

These are companion design notes to [KEP-2.18](../README.md), describing the concrete
design and implementation of `ConfigTemplate` and `Config` as first-class CRDs. The KEP
remains the baseline for direction; these notes capture the implementation-level design.
This index is the front door: read it first.

## Reading order

1. **[Design 01: CRD-based Config Management](./01-config-crd-management.md)**
   — the `config.oam.dev/v1alpha1` API group with `ConfigTemplate` and `Config` CRDs,
   their controllers, the validating webhook, and the dual-read / single-write backward
   compatibility with the legacy labeled `ConfigMap`/`Secret` convention.

## Status of each idea

| Topic | Status |
|---|---|
| CRD-based Config Management (Design 01) | Implementing |
