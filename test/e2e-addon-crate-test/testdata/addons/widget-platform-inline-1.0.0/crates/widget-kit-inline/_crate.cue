// Crate identity. Read by pkg/crate/parse.go (readCUEStringField): the two
// fields must be concrete strings at the top level; nothing else in this file
// is read. `crate` must be a DNS-1123 label (it becomes part of every
// installed definition name and of the owned Application name
// crate-<crate>). `version` must be strict semver (x.y.z, no leading v); it
// is the OCI tag `vela crate publish` pushes.
crate:  "widget-kit-inline"
version: "1.0.0"
