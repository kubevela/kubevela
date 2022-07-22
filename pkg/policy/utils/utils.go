package utils

// FilterComponents select the components using the selectors
func FilterComponents(components []string, selector []string) []string {
	if selector != nil {
		filter := map[string]bool{}
		for _, compName := range selector {
			filter[compName] = true
		}
		var _comps []string
		for _, compName := range components {
			if _, ok := filter[compName]; ok {
				_comps = append(_comps, compName)
			}
		}
		return _comps
	}
	return components
}
