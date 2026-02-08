# CUE Upgrade System

This package provides an extensible system for upgrading CUE templates to handle version compatibility as KubeVela evolves.

## Architecture

- **`upgrade.go`**: Core interface, registry, and main `Upgrade()` function
- **`upgrade_1_11.go`**: KubeVela 1.11+ specific upgrades (list concatenation)
- **`upgrade_test.go`**: Comprehensive test suite
- **Future**: `upgrade_1_12.go`, `upgrade_1_13.go`, etc.

## Usage

```go
import "github.com/oam-dev/kubevela/pkg/cue/upgrade"

// Upgrade using current KubeVela CLI version (auto-detected)
result, err := upgrade.Upgrade(cueTemplate)
if err != nil {
    // If version detection fails, you'll get a helpful error message
    // suggesting to use the --target-version flag
    log.Fatal(err)
}

// Upgrade to specific KubeVela version
result, err := upgrade.Upgrade(cueTemplate, "1.11")

// Future: upgrade to newer versions
result, err := upgrade.Upgrade(cueTemplate, "1.12")
```

## Adding New Version Upgrades

To add support for KubeVela 1.12 (example):

1. **Create** `upgrade_1_12.go`:

```go
package upgrade

import "fmt"

func init() {
    // Register your new upgrade functions
    RegisterUpgrade("1.12", upgradeSomeNewFeature)
    RegisterUpgrade("1.12", upgradeAnotherFeature)
}

// upgradeSomeNewFeature handles a breaking change in CUE 0.15
func upgradeSomeNewFeature(cueStr string) (string, error) {
    // Your upgrade logic here
    // Parse, transform, format, return
    return transformedCue, nil
}
```

2. **Update** `upgrade.go` `supportedVersions`:

```go
// In the Upgrade() function
supportedVersions := []string{"1.11", "1.12"}
```

3. **Add tests** to `upgrade_test.go` for the new functionality

4. **Done!** The system automatically applies all relevant upgrades

## Current Upgrades

### 1.11 - List Concatenation
- **Problem**: `list1 + list2` syntax deprecated in underlying CUE version
- **Solution**: Converts to `list.Concat([list1, list2])`
- **Auto-imports**: Adds `import "list"` when needed

## Features

- **Version-aware**: Only applies upgrades up to target version
- **Composable**: Multiple upgrades can be registered per version  
- **Safe**: Falls back gracefully on errors
- **Extensible**: Easy to add new version support
- **Well-tested**: Comprehensive test coverage

## Examples

### Before (Old CUE):
```cue
myList1: [1, 2, 3]
myList2: [4, 5, 6]
combined: myList1 + myList2
```

### After (CUE 0.14+):
```cue
import "list"

myList1: [1, 2, 3]
myList2: [4, 5, 6] 
combined: list.Concat([myList1, myList2])
```

This system ensures KubeVela templates remain compatible as CUE continues to evolve.

## CLI Usage

The upgrade functionality is available through the KubeVela CLI:

```bash
# Upgrade using current KubeVela CLI version (auto-detected)
vela def upgrade my-definition.cue

# Save upgraded definition to a file
vela def upgrade my-definition.cue -o upgraded-definition.cue

# Upgrade for specific KubeVela version
vela def upgrade my-definition.cue --target-version=v1.11

# Also works with just the version number
vela def upgrade my-definition.cue --target-version=1.11
```

**Version Detection:**
- When no `--target-version` is specified, the system automatically detects the current KubeVela CLI version
- If version detection fails (e.g., in development builds), you'll get a helpful error message suggesting to use `--target-version=1.11`
- **Note:** Use `--target-version=` (with equals sign) for the version flag