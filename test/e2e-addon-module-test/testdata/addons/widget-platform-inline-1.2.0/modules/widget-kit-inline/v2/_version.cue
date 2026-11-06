// widget-kit-inline 1.2.0 switches the v2 line off. Installing 1.2.0 over 1.0.0/1.1.0
// removes the v2 tiers (widget-kit-inline-v2-aux, widget-kit-inline-v2-defs) from the owned
// Application, so the v2 definitions and the v2 auxiliary objects are
// garbage-collected. The files stay in the artifact.
apiVersion: "v2"
enabled:    false
