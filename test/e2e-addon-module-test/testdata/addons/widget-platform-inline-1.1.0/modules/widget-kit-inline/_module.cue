// Module identity. Read by pkg/module/parse.go (readCUEStringField): the two
// fields must be concrete strings at the top level; nothing else in this file
// is read. `module` must be a DNS-1123 label (it becomes part of every
// installed definition name and of the owned Application name
// module-<module>). `version` must be strict semver (x.y.z, no leading v); it
// is the OCI tag `vela module publish` pushes.
module:  "widget-kit-inline"
version: "1.1.0"
