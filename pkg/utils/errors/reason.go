package errors

import "strings"

const (
	LabelConflict = "LabelConflict"
)

// IsLabelConflict checks if the error is Label Conflict error
func IsLabelConflict(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), LabelConflict) {
		return true
	}
	return false
}
