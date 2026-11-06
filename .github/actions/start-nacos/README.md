# Start Nacos Action

This GitHub composite action starts a standalone Nacos server for the config e2e suites. Run it after the KinD cluster exists.

## What it does

- Runs `nacos/nacos-server` on the `kind` Docker network, publishing ports 8848 and 9848 on the runner
- Waits until the server is ready, printing its log if it never is
- Exports `NACOS_ADDR` (how the suite on the runner reaches it) and `NACOS_POD_HOST` (how the controller's pods reach it)

The specs of `test/e2e-config-test` that need Nacos skip when these are not set; the config upgrade job requires them, and sets `NACOS_CLI_HOST` itself for the old CLI.
