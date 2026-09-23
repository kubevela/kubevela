# The context registry

`context.cue` declares every field the render context can carry, its type, and
which call sites offer it. Go reads this file rather than restating it, so
admission and render cannot disagree about what `context.x` means.

## Files

| | |
|---|---|
| `context.cue` | the registry: field groups, composed into one type per surface |
| `registry.go` | loads it (`//go:embed`); `ContextFor`, `SurfaceOffers`, `SurfaceDeclared`, `SurfacePlural`, `SurfaceNames` |
| `context.go` | `ContextSchema`, a view over one surface's `cue.Value` |
| `expr.go` | parsing a property value into literal and expression fragments |
| `reference.go` | the shape of a read, shared by the parser and the type checker |
| `optional.go` | which reads could be absent at render and carry no default |
| `walk.go` | traversing a properties blob, since every string in it may hold an expression |
| `helpers.go` | what survives the expression engine, given a source's schema is CUE |

## Shape

Fields are declared once in a group; groups compose into a type per call site.
The surface types are CUE types, so a field's type is its CUE type.

```cue
#ComponentIdentity: {componentName: string, componentType: string, ...}
#TraitIdentity:     {traitType: string}

surfaces: {
	component:    {#AppIdentity, #DeliveryIdentity, #ClusterIdentity, #ComponentIdentity, name: string}
	trait:        {surfaces.component, #TraitIdentity}
	workflowstep: {#AppIdentity, #DeliveryIdentity, #ClusterIdentity, #StepIdentity, name: string}
}
```

| Surface | Is |
|---|---|
| `component` | a ComponentDefinition template, and the properties substituted before it |
| `trait` | a TraitDefinition template: the component's context plus its own type |
| `workflowstep` | a workflow step's properties |
| `policy-rendered` | a PolicyDefinition with a CUE template |
| `policy-default` | a built-in policy (`topology`, `override`), read off the appfile |
| `policy-app` | an Application-scoped policy, rendered before the appfile exists |

Which of these may resolve a source is decided by `sourceReadingSurfaces` in
`pkg/sources/source_surfaces.go`, not here. That list also contains `source`,
for one source reading another, which is not a registry surface: a chained
source resolves inside whichever render triggered it and adds no context.

## Adding a field

1. Put it in the group that describes when it exists. It appears on every surface
   embedding that group. A field the render carries but nothing may read goes in
   `excluded:` with a `+reason=`; the loader panics without one.
2. Check the render path supplies it. A declared field the render omits passes
   admission and fails at render; one supplied as a permanent empty string passes
   both and tells the author nothing.
3. Run the tests, which are the enforcement:

   | Test | Enforces |
   |---|---|
   | `TestContextTypesMatchTheRenderContext` | every field the render carries is declared or excluded |
   | `TestRenderedPolicySurfaceMatchesTheRender` (`pkg/appfile`) | every declared field renders non-empty |
   | `TestKeyedFieldsExistInTheContextRegistry` (`pkg/definition/cachekey`) | every keyed field exists here |

   They unify a real render context with the declared type rather than comparing
   field kinds, so a new field cannot be missed:

   ```
   surfaces.component & <a real render context>
   → appRevisionNum: conflicting values "3" and int (mismatched types string and int)
   ```

4. To let a *source template* read it too, add it to the cache-key rules as well.
   See [`../cachekey/README.md`](../cachekey/README.md). This registry governs
   `$(context.x)` in an Application's properties; the rules govern `context.x`
   inside a SourceDefinition's template.

## Notes

- `context.custom` is `_`: whatever an Application-scoped policy published, absent
  unless one did. Reading it needs a type assertion and a default.
- `context.name` differs per surface (the component, the step, the binding), which
  is why each kind also gets `{kind}Name` and `{kind}Type`.
- An unknown surface falls back to the component's context rather than rejecting
  the expression.
