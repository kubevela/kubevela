package utils

// GatherErr will gather error in the object
type GatherErr []error

func (g GatherErr) Error() string {
	if len(g) == 0 {
		return ""
	}
	var ret string
	for _, v := range g {
		ret += v.Error() + "\n"
	}
	return ret
}
