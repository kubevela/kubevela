// _module.cue is the module identity file. It sits at the module root and is
// the first thing pkg/module.ParseModule reads.
//
// Only two fields are parsed. Everything else in this file is documentation
// for humans and is carried inside the published artifact untouched.

// module (REQUIRED, parsed) is the module's identity.
//
// It becomes the Helm chart name of the published OCI artifact, the OCI
// repository segment it is pushed to, the "modules.oam.dev/module" annotation
// on the artifact, the "definition.oam.dev/module" label on every definition
// this module installs, the name of the owned Application
// ("module-<module>"), and the first segment of every installed definition
// name ("<module>-<apiVersion>-<capability>").
//
// The fetch strips a "<module>/" prefix off the files it pulls back, so this
// must match the directory the module is published from or the module reads
// back as empty. Must be a DNS-1123 label.
module: "demo-store"

// version (REQUIRED, parsed) is the module's own version.
//
// Must be strict semver: major.minor.patch, no leading "v", no partials like
// "1.2". It is the tag `vela module publish` writes the artifact under, and
// it lands on the owned Application as the
// "definition.oam.dev/module-version" annotation.
//
// A published version is immutable. To ship a change, bump this and publish
// again. (`--version` on publish overrides the tag for a throwaway build; it
// does NOT change this value inside the artifact, and it does not bypass the
// immutability check unless --force is also passed.)
version: "1.0.0"

// description (NOT parsed) is free-form prose for humans reading the source.
// The published Chart.yaml carries a generated description instead.
description: "Demo module exercising every supported module feature: multiple API lines, multiple definition kinds, module-level and line-level auxiliary resources."

// owners (NOT parsed) is free-form. Kept because `vela module init`
// scaffolds it and teams use it to record who to page.
owners: ["kubevela-localtest"]
