# Definition Versioning

// ...existing content...

### Definition Version Immutability

Once a definition is created with a specific version (e.g., `1.0.0`), it cannot be updated without changing the version. This ensures that applications referencing a specific version remain consistent. KubeVela enforces this behavior using a validating webhook. If you attempt to update a definition with the same version, the request will be rejected.

**Example error:**
```
Error: admission webhook "validate-componentdefinition.kubevela.io" denied the request: Definition with version 1.0.0 is immutable and cannot be updated
```

To update a definition, increment the `spec.version` field.

