package cuegen

// RegisterAny registers go types' package+name as any type({...} in CUE)
//
// Example:RegisterAny("*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured")
//
// Default any types are: map[string]interface{}, map[string]any, interface{}, any
func (g *Generator) RegisterAny(types ...string) {
	for _, t := range types {
		g.anyTypes[t] = struct{}{}
	}
}
