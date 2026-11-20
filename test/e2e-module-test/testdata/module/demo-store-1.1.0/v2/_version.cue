// The second API line. It is a sibling of v1, not a successor: both install
// at the same time from the same artifact, and neither waits on the other.
//
// This is how a module ships a breaking change without breaking existing
// Applications. An Application pinned to "demo-store/v1/bucket" keeps getting
// the v1 definition; one asking for "demo-store/v2/bucket" gets this one.
// Retire a line by setting enabled: false here and publishing a new version.
apiVersion: "v2"
enabled:    true
