// A disabled line.
//
// It is packaged into the published artifact and appears in the
// "modules.oam.dev/lines" annotation, so it travels with the module and can
// be turned on later by flipping this flag and publishing a new version. It
// is left out of "modules.oam.dev/enabled-lines", the render service skips
// it, and none of its definitions or auxiliary resources are installed.
//
// This is also the shape a retired line takes: the definitions stay in the
// source tree and in the artifact, but stop being installed.
apiVersion: "v1alpha1"
enabled:    false
