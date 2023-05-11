package common

// GetNamespaceFromConfig returns namespace from kubeconfig, if not found, return empty string
func GetNamespaceFromConfig() string {
	conf := RawConfigOrNil()
	if conf == nil || conf.Contexts == nil {
		return ""
	}
	ctx, ok := conf.Contexts[conf.CurrentContext]
	if !ok {
		return ""
	}
	return ctx.Namespace
}
