## What changed

1. Added a defensive nil-guard on the manifest returned by `ApplyStrategies` in the **dispatch** code path (`pkg/resourcekeeper/dispatch.go`). Previously, only the StateKeep path checked for a nil manifest.

2. After StateKeep determines that an apply-once resource no longer exists and should not be re-created, the stale entry is now removed from the `ResourceTracker` and the update is persisted to the API server. This prevents future StateKeep cycles from iterating over stale entries and issuing unnecessary GET requests.

## Why

The nil-guard in the dispatch path makes the contract between `ApplyStrategies` and its callers more robust — if a future change introduces another nil-return path, the dispatch code won't panic.

The stale entry cleanup reduces unnecessary API server load and log noise that would otherwise accumulate as externally deleted apply-once resources leave behind orphaned ResourceTracker entries.

Fixes #7055

## Testing

- Updated the existing envtest-based test `"Test StateKeep apply-once does not re-create externally deleted resource"` to also verify that the stale ResourceTracker entry is removed after StateKeep completes.
- Verified `go build` and `go vet` pass cleanly.
- All existing unit tests pass.
