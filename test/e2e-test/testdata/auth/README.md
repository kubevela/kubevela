# E2E auth test fixtures

Committed artifacts used by `Describe("Helmchart Auth")` in
`test/e2e-test/helmchart_test.go`. All files are test-only.

## What's here

- `htpasswd` - bcrypt of `test-user:test-pass`, used by zot, chartmuseum, and nginx-bearer.
- `certs/{ca.crt,server.crt,server.key}` - self-signed CA + server cert valid
  for `*.kubevela-auth-test.svc.cluster.local`. CA is used by client-side
  TLS-Secret tests; server cert is mounted into the registry pods.
- `chart/podinfo-test-1.0.0.tgz` - minimal podinfo chart (~5 KB) pushed to
  each registry by `BeforeSuite`.
- `chart/source/` - chart source for reproducibility.
- `nginx.conf` - bearer-validating reverse proxy config; checks
  `Authorization: Bearer kubevela-auth-test-token` and proxies to the
  ChartMuseum pod behind.
- `manifests/` - Deployments, Services, Secrets, ConfigMaps for the
  three registries. Applied by `setupAuthRegistries`.
- `apps/` - one Application YAML per scenario.
- `scripts/regenerate.sh` - regenerates `htpasswd`, certs, and chart
  tarball. Run by hand when creds, certs, or chart shape change. The
  script prefers a local `htpasswd` binary but falls back to
  `docker run --rm httpd:2-alpine htpasswd ...` when the binary is
  not installed (this devcontainer's default).

## Static bearer token

The bearer-front uses a fixed static token: `kubevela-auth-test-token`.
This is test-only; it never leaves the kubevela-auth-test namespace.
