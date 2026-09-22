# Source Cache-Key Rules

`rules/a-v1.cue` decides two things at once:

1. **What a SourceDefinition's CUE template may read** from `context`. Anything not
   listed is rejected at `vela def apply` and is *absent* from the context a source
   is compiled against — so it cannot be read even where admission is disabled.
2. **How a cache key is assembled** from those reads.

A source's key is never authored. `Infer` derives it from what the template
actually reads, and `Stamp` writes the result into the definition's `$internal:`
block. Admission re-derives it and rejects a mismatch, so the block cannot be
edited by hand.

For the *other* half of context — what an Application's `$(context.x)` expressions
may read — see [`../sourceexpr/README.md`](../sourceexpr/README.md). The two files
are routinely confused; that README opens with the distinction.

## Architecture

- **`rules/a-v1.cue`** — the rules. One file per version; all are embedded.
- **`rules.go`** — loads and hashes them (`LoadRules`, `LoadRulesByHash`, `policyHash`).
- **`infer.go`** — derives the key dimensions from a template's reads.
- **`stamp.go`** — writes the `$internal:` block.
- **`expression.go`** — assembles the key expression from the dimensions.
- **`surfaces.go`** — which surfaces can supply a set of fields (`SurfacesSupporting`,
  `CheckSurface`, `MissingOn`).
- **`key_validation.go`** — validates a stamped key against a re-derivation.

## The assembled identity

```
<definition>[-<readable segments...>]-<hash>
```

The **hash covers every value the template reads** and carries uniqueness on its
own. Segments are cosmetic — they exist so an operator can grep, and an empty one
is simply dropped. `segment: true` therefore says nothing about correctness.

```cue
keyed: {
	cluster: {order: 2, segment: true}     // inlined and hashed
	clusterVersion: order: 3               // hashed only - a struct has no rendered form
	appLabels: {order: 10, indexed: true}  // read by literal key: appLabels["team"]
}
```

| Option | Means |
|---|---|
| `order` | position in the key; dimensions come out in this sequence, not alphabetically |
| `segment: true` | inline the value in the readable prefix as well as hashing it |
| `indexed: true` | the read carries a literal key, and that key's value is what contributes |

Inline only what is **always renderable** — a Kubernetes-style name. Not a struct
(`clusterVersion`), not a free-form value (label and annotation values may contain
characters legal there and illegal in an object name), and not something
legitimately empty in normal operation (`appRevision` before the first revision,
`replicaKey` outside the replication policy), which would leave a blank segment.

## Readable implies keyed

There is no "a template may read this, but it does not affect the key". If a
template reads a value, that value **must** contribute, or two different values
share one cache entry and the cache is wrong.

What you can choose is whether it is *inlined*. A hash-only field separates entries
correctly without lengthening the key. That is the right answer for anything not
worth grepping.

A read costs a cache entry per distinct value. That is the cost of *reading* one,
not of listing it here — a template that does not read a field is unaffected by
its presence.

## Changing the rules

Before release the version rule does not bite: nothing outside this repository has
ever been stamped, so **edit `a-v1.cue` in place** and regenerate. After release,
add a new file alongside it — a definition stamped with one version's hash loads
that version's rules, so the old file must remain for as long as any definition
references it.

To edit in place:

1. **Edit `keyed:`.** Give the field an `order`, and `segment: true` only if it is
   always renderable and worth grepping.

2. **Run the tests.** `TestStampedRulesHashHasNotMoved` will fail and tell you the
   new hash. It exists so an accidental edit is a failing test rather than a
   mismatch discovered against a cluster.

3. **Restamp every generated definition.** The stamped annotation is
   `definition.oam.dev/cache-key-rules`:

   ```bash
   grep -rl '<old-hash>' examples/ | xargs sed -i '' 's/<old-hash>/<new-hash>/g'
   ```

   Adding fields does not change existing keys: a template that does not read the
   new field keeps its `$internal:` block byte-for-byte. Only the annotation moves.

4. **Update the pinned constant** in `TestStampedRulesHashHasNotMoved`.

5. **Apply every definition against a running controller.** This is the real proof
   — admission re-derives the stamp and rejects a mismatch, so acceptance means the
   restamp is correct:

   ```bash
   for f in charts/vela-core/templates/defwithtemplate/*.yaml; do kubectl apply -f "$f"; done
   ```

Comments do not affect the hash — it covers the decoded `keyed` map alone — so
documenting a field is free.

## A keyed field must exist in the registry

`TestKeyedFieldsExistInTheContextRegistry` enforces two things: the field is
declared in `sourceexpr/context.cue`, and it is offered by **at least one** surface
that resolves a source.

At least one, not all. A field offered by only some surfaces is the point — it is
what restricts where a source reading it may be consumed:

| Field | A source reading it is consumable from |
|---|---|
| `appName`, `namespace`, `cluster`, … | everywhere |
| `componentName`, `componentType` | components, traits |
| `traitType` | traits |
| `stepName`, `stepType` | workflow steps |
| `policyName`, `policyType` | policies that render resources |

Zero surfaces is the error: the field would be absent at render on every path, so
no source could ever use it. `policyRevisionName` is the shape of that mistake —
it is in the registry, but only on `policy-app`, which renders before the appfile
exists and so resolves no sources.

That restriction is enforced per binding at Application admission and shown by
`vela def show` before anything is applied.

## What the tests enforce

| Test | Enforces |
|---|---|
| `TestStampedRulesHashHasNotMoved` | the hash matches what generated definitions carry |
| `TestKeyedFieldsExistInTheContextRegistry` | every keyed field exists and is reachable |
| `TestKeyExpressionInlinesOnlySegments` | only `segment: true` fields reach the readable prefix |
| `TestKeyInputsRecordEveryDimension` | `$internal.keyInputs` matches the inferred dimensions |
| `TestUnsupportedContextIsOneMessage` | an unlisted field is rejected by name, pointing at properties |
| `TestSurfaceSpecificFieldsRestrict` | surface-specific fields genuinely narrow consumption |
