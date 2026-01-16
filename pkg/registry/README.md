# Provider Registry

A minimal, interface-based provider registry for breaking import cycles in the codebase.

## Purpose

This registry is a **fallback mechanism** for situations where import cycles block development work. It provides runtime indirection through interface-based contracts while maintaining type safety.

## Why It Exists

Large, mature codebases sometimes develop import cycles between packages:
- `pkg/appfile` needs types from `pkg/controller`
- `pkg/controller` needs functionality from `pkg/appfile`
- Result: Import cycle prevents compilation

While the ideal solution is restructuring packages with clear boundaries, this isn't always practical in the short term. The registry unblocks development while allowing refactoring efforts to be planned appropriately.

## Design Philosophy

**Use as a fallback, not a default:**
- New code should use well-designed package boundaries and constructor injection
- Existing code can use the registry when cycles genuinely block work
- Services registered here are candidates for future refactoring

**Keep it simple:**
- Interface-only registration (enforced)
- Thread-safe operations
- No complex lifecycle management
- Stdlib dependencies only

## How It Works

### 1. Define an Interface

```go
// In cmd/core/app/bootstrap.go or appropriate location
type MyProvider interface {
    DoSomething(ctx context.Context) error
}
```

### 2. Register During Bootstrap

```go
// In cmd/core/app/bootstrap.go
func bootstrapProviderRegistry() {
    // MyProvider - Brief description of what it does
    // Cycle: pkg/foo ↔ pkg/bar
    // Note: Consider refactoring to extract shared interfaces
    provider := foo.NewMyProvider()
    registry.RegisterAs[MyProvider](provider)
}
```

### 3. Retrieve Where Needed

```go
// In any package that needs it
provider, ok := registry.Get[MyProvider]()
if !ok {
    return fmt.Errorf("MyProvider not registered")
}
err := provider.DoSomething(ctx)
```

## API

- **`RegisterAs[T any](impl T)`** - Register an implementation for interface T
- **`Get[T any]() (T, bool)`** - Retrieve registered implementation
- **`Snapshot() RegistrySnapshot`** - Save current state (for testing)
- **`Restore(snapshot RegistrySnapshot)`** - Restore saved state (for testing)

## Testing

Use Snapshot/Restore to isolate tests:

```go
func TestSomething(t *testing.T) {
    snapshot := registry.Snapshot()
    defer registry.Restore(snapshot)

    // Override with mock
    registry.RegisterAs[MyProvider](mockImpl)

    // Test code using mock
}
// Original providers restored automatically
```

## When NOT to Use

Prefer constructor injection when:
- Writing new code with clean package boundaries
- No import cycle exists between packages
- Extracting interfaces to a neutral package is straightforward
- Dependencies can be passed explicitly through constructors

## Trade-offs

**Benefits:**
- Unblocks development immediately
- Breaks cycles without major refactoring
- Type-safe through generics
- Easy to mock in tests

**Costs:**
- Less visible than constructor injection
- Runtime lookups instead of compile-time
- Can hide architectural issues if overused

## Guidance

Use this registry judiciously as a pragmatic tool. Each provider registered represents an opportunity to improve package structure in the future. The goal is to keep usage minimal while unblocking important development work.

For detailed implementation guidelines, see the package documentation in `registry.go`.
