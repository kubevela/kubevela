# Design 09: Known limits

Companion design note to [KEP-2.18](../README.md). See the [design index](./README.md).

- Owner: Jerrin Francis (@jerrinfrancis)
- Date: 2026-11-16
- Status: Open

### Introduction

What the implementation described in Designs 01 to 08 does not do yet, as of KubeVela v1.12. Each
item is a candidate for a follow-up; none blocks the upgrade or migration paths.

### Status and observability

| Limit | Effect | Possible fix |
|---|---|---|
| No `Ready` condition on `Config` or `ConfigTemplate` | Flux and Argo CD report a Config in `Error` as healthy | a `Ready` condition (or kstatus-compatible status) both tools read |
| The Config controller emits no Events | failures show only on `status`; `kubectl describe` has no history | record Events on `Error` and on Nacos publishes |
| `NacosSynced` can read `AlreadyCurrent` right after the controller published | the reason understates what happened; the content is correct | tell a read that follows the controller's own publish apart from a first sync |
| `vela config create` and `vela config-template apply` print success before the CR is `Available` | a bad render shows only on the CR's status | an optional wait, as the `create-config` step does |

### Reconciliation

| Limit | Effect | Possible fix |
|---|---|---|
| `outputs` objects other than Secrets are not watched | one deleted or edited by hand returns only on the Config's next reconcile | watch the kinds a template's `outputs` use |
| An entry dropped from a template's `outputs`, or an output Secret whose rendered name changes, leaves the old object | it stays, owned by the Config, until the Config is deleted (1.11 left it too, untracked) | prune owned objects the last render did not produce |
| A template turned sensitive in the same edit that breaks its CUE cannot be resolved | its Configs' Secrets are not marked sensitive until the template is fixed; `read-config` still refuses them because the Configs are in `Error` | mark the Secret from the template's `spec.sensitive` even when the template is not usable |
| The Config webhook reads the `propertiesFrom` Secret through the manager's cache | a Config applied right after its Secret can be refused until the cache sees the Secret; a retry succeeds | read it through the API reader |

### Admission with the webhooks off

| Limit | Effect |
|---|---|
| On Kubernetes older than 1.25, `sensitive` can be turned off | the CEL rule is ignored there; Secrets already marked stay sensitive, but inline properties are accepted again for new Configs |
| No template access review | the SubjectAccessReview needs a requesting user, which only admission has ([Design 04](./04-template-scope-and-access.md)) |

### Migration

| Limit | Effect | Possible fix |
|---|---|---|
| No watch for legacy objects created after the startup sweep | a config an old CLI writes later stays legacy | rerun `migrate`, or restart the controller with the gate on; event-driven adoption if needed |
| A dry run reports a config bound across namespaces as `skipped`; the real run adopts it | the dry run understates what will be adopted | evaluate the grandfathered bindings in the dry run as in the real run |
| Addons install config templates as legacy ConfigMaps | once migrated, an addon upgrade changes only the ConfigMap while the CR keeps the old template; rerunning `migrate` does not update the CR | addons write `ConfigTemplate` CRs, or migration updates a CR it created when its source changed; meanwhile delete the CR and migrate again |
| Legacy objects are never deleted | they stay after adoption, for rollback | an explicit `migrate --delete-source` once rollback is no longer wanted |

### CLI and templates

| Limit | Effect |
|---|---|
| A template's `metadata.description` is dropped when it is parsed | `vela config-template apply` writes an empty `spec.description` (1.11 lost it the same way) |
| The 1.11 CLI cannot create a config from a template that exists only as a CR | use the new CLI, or keep the template legacy until clients are upgraded |
| The 1.11 CLI's `config create -f file` reads numbers as floats | an `int` parameter fails; the argument form (`key=3306`) works |

### Chart

| Limit | Effect |
|---|---|
| The controller's RBAC is broad, and the chart ships a `template-reader` Role and binding nothing uses | access is decided per user at admission ([Design 04](./04-template-scope-and-access.md)); the RBAC could be narrowed |

### By design, not limits

- Deleting a Config leaves its Nacos entry ([Design 06](./06-nacos-expanded-writer.md)).
- The legacy storage keeps being read, with no removal date ([Design 02](./02-backward-compatibility-and-cli.md)).
- Distribution keeps the 1.11 mechanism; the KEP's `spec.topology` model is future work.
