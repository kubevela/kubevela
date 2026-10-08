// An addon-level definition (not part of any module). Installed under its own
// name, platform-owner, in vela-system, through the addon-definitions
// component. It carries no module labels, so it resolves the legacy way.
"platform-owner": {
	type:        "trait"
	description: "Annotates the workload with the owning team (widget-platform addon)."
	attributes: podDisruptive: false
}
template: {
	patch: metadata: annotations: "kit.example.com/owner": parameter.team
	parameter: team: *"platform" | string
}
