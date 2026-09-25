// _version.cue is the API line identity file. Every directory at the module
// root whose name starts with "v" is read as an API line, and this file is
// required in each of them.
//
// Both fields below are parsed. Nothing else in this file is.

// apiVersion (REQUIRED, parsed) is the line's identity.
//
// It must match ^v\d+(alpha\d+|beta\d+)?$ -- v1, v2, v1beta1, v2alpha1. It is
// the middle segment of every definition name this line installs
// ("<module>-<apiVersion>-<capability>") and the value of the
// "definition.oam.dev/module-api-version" label on them.
//
// It does not have to equal the directory name, but keeping them identical is
// what every other tool assumes; two directories resolving to the same
// apiVersion is a parse error.
apiVersion: "v1"

// enabled (OPTIONAL, parsed, defaults to true) is the author's install switch
// for this line.
//
// An enabled line is installed by the render service. A disabled line is
// still packaged into the artifact and still listed in the
// "modules.oam.dev/lines" annotation, but is left out of
// "modules.oam.dev/enabled-lines" and installs nothing. See v1alpha1/ in this
// module for a disabled line.
enabled: true
