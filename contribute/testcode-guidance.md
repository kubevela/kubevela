# Principle of Test


Please refer to https://kubevela.io/docs/contributor/principle-of-test for details.

## Running unit tests locally

The unit tests require a kubeconfig to be present, even though they never
contact a real cluster: several packages resolve Kubernetes clients through
`github.com/kubevela/pkg/util/singleton`, whose loader exits the process when
no kubeconfig can be found. Affected packages guard for this in `TestMain` and
fail with instructions instead of dying silently.

If you have no cluster configured, any parseable kubeconfig is enough:

```shell
mkdir -p ~/.kube && cat > ~/.kube/config <<'EOF'
apiVersion: v1
kind: Config
clusters:
- cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}
  name: dummy
contexts:
- context: {cluster: dummy, user: dummy}
  name: dummy
current-context: dummy
users:
- name: dummy
  user: {token: dummy}
EOF
```

Some suites additionally need the envtest binaries
(`setup-envtest use 1.31.0`, exported via `KUBEBUILDER_ASSETS`), and
`pkg/definition/gen_sdk` needs a running Docker daemon.