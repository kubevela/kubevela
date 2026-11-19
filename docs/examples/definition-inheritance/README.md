# Definition inheritance: every rule, one file per rule

A ComponentDefinition or TraitDefinition with `spec.extends: <parent>` is its parent with more on it. The child passes the parent its parameters through `$super: properties`, the parent renders, and the child's own fields are added on top.

Turn the feature on first. It is alpha and off by default:

```bash
helm upgrade --install kubevela kubevela/vela-core -n vela-system --create-namespace \
  --set featureGates.enableDefinitionInheritance=true \
  --set featureGates.validateResourcesExist=true
```

`validateResourcesExist` is only needed for the last workload case in `6-workload.yaml` (F09), which an older check refuses.

## The rules

| A definition that extends another may | It may not |
|---|---|
| add fields its parent did not set, or fill one the parent left open | change a field its parent set, even one from a defaulted parameter |
| change a parent value through `$super: properties` | replace what the parent renders: `$inherit: false`, `{output: false}`, `{outputs: false}`, `{patch: false}` |
| leave out one named output, `$inherit: {outputs: {<name>: false}}`, and write its own | leave out an output the parent marks `// +required` |
| add outputs of its own | add anything to a field the parent marks `// +final` |
| leave `spec.workload` unset to inherit it | declare a workload other than its parent's, or other than what the root renders |

A parent's `// +immutable` parameter also holds through a child that forwards or renames it. Every rule is checked when a definition is written and again when an Application renders it.

## The files

Each file is a sequence of objects in the order to apply them. Every object starts with a comment that says what should happen to it: `accepted`, or `refused, with: <the start of the error>`.

| File | Shows |
|---|---|
| `0-what-still-works.yaml` | a three-level chain, values passed up through `$super`, a child reading `$super.output`, inherited and combined health and status, an abstract base, the namespace rule, a cycle |
| `1-inherit-drops-one-output.yaml` | `$inherit` only drops single outputs; the whole-surface forms are refused |
| `2-no-override.yaml` | a child adds and never changes; changing replicas the right way through `$super` |
| `3-final.yaml` | `// +final` at any depth, set by any level, on a trait, and added after a child exists |
| `4-required.yaml` | `// +required` on one output, on the whole `outputs` block, and on a trait's `patchOutputs` |
| `5-status-reads.yaml` | a child cannot drop an output that an inherited `healthPolicy` or `customStatus` reads |
| `6-workload.yaml` | a child's `spec.workload` must match the parent's, or what an autodetect root renders |
| `7-immutable.yaml` | definitions and Applications for the `+immutable` updates below |
| `8-pinning-and-traits-1.yaml`, `-2.yaml` | a child pinned to `parent@v1`, and a trait extending a trait |

## Run it

```bash
for f in 0-*.yaml 1-*.yaml 2-*.yaml 3-*.yaml 4-*.yaml 5-*.yaml 6-*.yaml 7-*.yaml 8-*-1.yaml; do
  kubectl apply -f "$f"
done
kubectl -n vela-system wait --for=create definitionrevision/hp-base-v1 --timeout=60s
kubectl apply -f 8-pinning-and-traits-2.yaml
```

`kubectl` prints an error for each refused object and goes on with the rest of the file. Compare each error with the object's `# expect:` comment. `8-pinning-and-traits-1.yaml` must have its revision `v1` before the parent changes in `-2.yaml`.

Then look at what rendered, in namespace `default`:

| Check | Expected |
|---|---|
| `kubectl get deploy n-t -o jsonpath='{.metadata.labels.team} {.spec.replicas} {.spec.template.spec.containers[0].env[0].value}'` | `payments 2 acme` (a label from the child, replicas passed up, an env var added) |
| `kubectl get configmap n-t-info -o jsonpath='{.data.image}'` | the image the parent rendered, read through `$super.output` |
| `kubectl get deploy n-l -o jsonpath='{.metadata.labels.team} {.metadata.annotations.example\.com/owner}'` | `payments checkout` (one from each level above the root) |
| `kubectl get app n-app -o jsonpath='{.status.services[*].healthy}'` | `true true true` |
| `kubectl get apprev n-app-v1 -o jsonpath='{.spec.componentDefinitions.n-base.metadata.name}'` | `n-base` (the revision records the ancestors) |
| `kubectl get networkpolicy a-own -o jsonpath='{.spec.ingress}'` | `[{}]`, the child's own policy in place of the parent's |
| `kubectl get networkpolicy a-none` | not found; the child left it out |
| `kubectl get deploy b-three -o jsonpath='{.spec.replicas}'` | `3`, passed through `$super` |
| `kubectl get deploy c-labelled -o jsonpath='{.spec.template.spec.securityContext}'` | `{"runAsNonRoot":true}`, exactly the parent's |
| `kubectl get deploy hp-p -o jsonpath='{.metadata.labels.rev}'` | empty: pinned to `v1` |
| `kubectl get deploy hp-f -o jsonpath='{.metadata.labels.rev}'` | `v2`: follows the parent |
| `kubectl get deploy h-t -o jsonpath='{.metadata.labels.managed},{.metadata.labels.team}'` | `true,h`, a label from each trait |

### `+immutable` through a child

`g-volume` marks `storageClass` `// +immutable`. `g-forward` passes it on under the same name, `g-tiered` maps its own `tier` onto it, and `g-fixed` sets it itself. Update the Applications:

```bash
# refused: g-volume's immutable parameter storageClass cannot be changed (current: "fast", new: "slow")
kubectl patch app g-app --type=json -p '[{"op":"replace","path":"/spec/components/0/properties/storageClass","value":"slow"}]'
# refused the same way, although the field the Application changes is tier
kubectl patch app g-app --type=json -p '[{"op":"replace","path":"/spec/components/1/properties/tier","value":"slow"}]'
# accepted: size is not immutable
kubectl patch app g-app --type=json -p '[{"op":"add","path":"/spec/components/1/properties/size","value":"2Gi"}]'

# accepted: a parameter that only had its default is set for the first time
kubectl patch app g-rep-app --type=json -p '[{"op":"add","path":"/spec/components/0/properties","value":{"replicas":3}}]'
# refused: cannot be changed (current: 3, new: 4)
kubectl patch app g-rep-app --type=json -p '[{"op":"replace","path":"/spec/components/0/properties/replicas","value":4}]'
# refused: cannot be removed (current: 3)
kubectl patch app g-rep-app --type=json -p '[{"op":"remove","path":"/spec/components/0/properties/replicas"}]'
```

The `g-app` volumes stay `Pending` unless the cluster has StorageClasses named `fast` and `standard`. The updates are judged by the admission webhook either way.

## Clean up

```bash
kubectl delete app -n default n-app n-paved-app a-app b-app c-app e-app f-app g-app g-rep-app hp-app h-trait-app --ignore-not-found
for f in *.yaml; do kubectl delete -f "$f" --ignore-not-found; done
```
