package registry

// #ReadFile fetches a single file from a named addon registry.
//
// The registry is looked up in the cluster's registry ConfigMap, so a
// SourceDefinition names a registry the platform has already configured rather
// than carrying a URL and a token of its own. Whatever auth that registry was
// registered with is what the read uses.
#ReadFile: {
	#do:       "read-file"
	#provider: "registry"

	// +usage=The params of this action
	$params: {
		// +usage=Name of a registry configured in this cluster, as listed by `vela registry ls`
		registry: string
		// +usage=Path of the file within the registry, relative to the registry's own root path
		path: string
		// +usage=Branch, tag or commit to read at. Defaults to whatever the registry's URL pinned.
		ref?: string
	}

	// +usage=The file's contents
	$returns?: {
		// +usage=The file contents, verbatim. Empty when found is false.
		content: string
		// +usage=Whether the registry had the file. A missing file is reported here rather than as an error, so an optional file can be read and fallen back on.
		found: bool
		...
	}
	...
}

// #MustReadFile reads a file that has to be there.
//
// #ReadFile hands a missing file back as found: false, which is what an
// optional override wants. A required file wants the opposite - noticing at
// resolution time rather than rendering an empty value that looks like data -
// so this asserts found and lets the unification fail if the file is absent.
#MustReadFile: {
	$params: {
		registry: string
		path:     string
		ref?:     string
	}

	// Bound through a let: writing {$params: $params} would make the inner
	// $params refer to the field being defined, not to this one, and the read
	// would go out with the abstract schema instead of the caller's values.
	let call = $params

	_read: #ReadFile & {$params: call}

	// Fails the evaluation if the file was not found. The reason is carried
	// inside the conflict because CUE has no custom error construct, and a bare
	// "conflicting values false and true" says nothing about which file.
	_found: true & [
		if _read.$returns.found {true},
		"required file \"\(call.path)\" is not in registry \"\(call.registry)\"",
	][0]

	$returns: {
		content: _read.$returns.content
		found:   true
	}
}
