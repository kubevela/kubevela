# Workflow HTTP denylist config

The controller discovers every ConfigMap labeled
`config.oam.dev/type=vela-core-http-deny` and unions their entries on top of an
immutable builtin floor (link-local + cloud metadata) compiled into the workflow
binary. The Helm chart installs its own ConfigTemplate and a labeled default
ConfigMap with the same entries for Day 1; ConfigMaps can only add denies, not
remove the floor.

## Dual install with vela-workflow

`vela-core` and `vela-workflow` each own a **distinct** ConfigTemplate and
default ConfigMap so uninstalling one release does not delete the other's
template:

| Chart | ConfigTemplate / discovery label | Default ConfigMap |
|-------|----------------------------------|-------------------|
| vela-core | `vela-core-http-deny` | `vela-core-http-deny-default` |
| vela-workflow | `vela-workflow-http-deny` | `vela-workflow-http-deny-default` |

Each controller only merges ConfigMaps matching **its** discovery label (plus
the Go builtin floor).

Create additional denylists with the ConfigTemplate:

```bash
vela config create team-http-deny \
  --template vela-core-http-deny \
  --namespace vela-system \
  'denyHosts={*.example.com}' \
  'denyCIDRs={10.0.0.0/8}'
```

Disable discovery or the shipped default ConfigMap if needed:

```bash
--set workflow.httpDeny.configTemplateName=""
--set workflow.httpDeny.defaultConfig.enabled=false
```

Raw ConfigMaps remain supported when they carry:

```yaml
metadata:
  labels:
    config.oam.dev/type: vela-core-http-deny
data:
  denyHosts: |
    blocked.example
  denyCIDRs: |
    10.0.0.0/8
```
