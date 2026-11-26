// _crate.cue is the crate identity file. It sits at the crate root and is
// the first thing pkg/crate.ParseCrate reads.
//
// Only two fields are parsed. Everything else in this file is documentation
// for humans and is carried inside the published artifact untouched.

// crate (REQUIRED, parsed) is the crate's identity.
//
// It becomes the Helm chart name of the published OCI artifact, the OCI
// repository segment it is pushed to, the "crates.oam.dev/crate" annotation
// on the artifact, the "definition.oam.dev/crate" label on every definition
// this crate installs, the name of the owned Application
// ("crate-<crate>"), and the first segment of every installed definition
// name ("<crate>-<apiVersion>-<capability>").
//
// The fetch strips a "<crate>/" prefix off the files it pulls back, so this
// must match the directory the crate is published from or the crate reads
// back as empty. Must be a DNS-1123 label.
crate: "demo-store"

// version (REQUIRED, parsed) is the crate's own version.
//
// Must be strict semver: major.minor.patch, no leading "v", no partials like
// "1.2". It is the tag `vela crate publish` writes the artifact under, and
// it lands on the owned Application as the
// "definition.oam.dev/crate-version" annotation.
//
// A published version is immutable. To ship a change, bump this and publish
// again. (`--version` on publish overrides the tag for a throwaway build; it
// does NOT change this value inside the artifact, and it does not bypass the
// immutability check unless --force is also passed.)
version: "1.0.0"

// description (NOT parsed) is free-form prose for humans reading the source.
// The published Chart.yaml carries a generated description instead.
description: "Demo crate exercising every supported crate feature: multiple API lines, multiple definition kinds, crate-level and line-level auxiliary resources."

// owners (NOT parsed) is free-form. Kept because `vela crate init`
// scaffolds it and teams use it to record who to page.
owners: ["kubevela-localtest"]
