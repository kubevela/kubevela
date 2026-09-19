# CUE post-rendering on the `helmchart` component

`options.postRender.cue` lets you transform the manifests a Helm chart renders,
without forking the chart or waiting for it to expose a value you need.

The template is evaluated **once per rendered resource**, and runs **after** Helm
renders the chart but **before** KubeVela stamps its ownership labels. That
ordering is deliberate: a template cannot patch away the labels KubeVela uses to
track and garbage-collect the release.

```yaml
options:
  postRender:
    cue:
      template: |
        patch: {
        	if context.resource.kind == "Deployment" {
        		spec: template: spec: {
        			// +patchKey=name
        			containers: [{
        				name: "podinfo"
        				// +patchKey=name
        				env: [{name: "DEPLOY_ENV", value: "prod"}]
        			}]
        		}
        	}
        }
```

## What the template sees

| Field                  | Value                                              |
| ---------------------- | -------------------------------------------------- |
| `context.resource`     | the resource currently being patched, as rendered   |
| `context.appName`      | the Application name                                |
| `context.appNamespace` | the Application namespace                           |
| `context.name`         | the component name                                  |
| `context.namespace`    | the component namespace                             |

The full CUE standard library (`strings`, `list`, `regexp`, ...) is available.
Vela provider functions (`vela/kube`, `vela/http`, ...) are not: a post-render
template is a pure transform over an already-rendered resource.

## What the template produces

Emit a `patch` field. It is merged into the resource using the same
strategy-unify semantics traits use to patch workloads.

**Targeting a subset of resources.** The template runs against every rendered
resource, so guard it on whatever identifies your target. Producing no `patch`
field at all leaves that resource untouched:

```cue
patch: {
	if context.resource.kind == "Deployment" {
		// only Deployments are patched; everything else passes through
	}
}
```

**Merging into lists.** Plain unification pairs list entries by position, which
is rarely what you want for containers, env vars, or ports. Annotate the list
with `// +patchKey=<field>` to match entries by a field instead, and leave the
entries you are not changing out entirely:

```cue
// +patchKey=name
env: [{name: "DEPLOY_ENV", value: "prod"}]
```

Use `// +patchStrategy=replace` when you really do want to discard whatever the
chart rendered and substitute your own list.

**Overriding a value the chart already set.** CUE unification can only make a
value *more* specific, so patching `replicas: 3` over a chart that rendered
`replicas: 1` is a conflict, not an override. Ask for the override explicitly:

```cue
spec: {
	// +patchStrategy=retainKeys
	replicas: 3
}
```

This is the one sharp edge worth internalizing. It fails loudly with
`conflicting values 3 and 1` rather than silently keeping the chart's value, so
you will find out at render time rather than in production.

## Examples

| File                        | Shows                                                        |
| --------------------------- | ------------------------------------------------------------ |
| `inject-env.yaml`           | adding an env var to one container via `patchKey`             |
| `annotate-and-scale.yaml`   | annotating every resource, plus a `retainKeys` replica override |

Both render against the public `podinfo` chart:

```bash
vela dry-run -f inject-env.yaml
```

## Ordering

When other post-render flavors are configured alongside CUE, user
transformations run first and KubeVela's ownership labelling always runs last.
