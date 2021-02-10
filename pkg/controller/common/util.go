package common

import (
	"fmt"
	"strings"
)

// ConstructRevisionName will generate revisionName from componentName
// will be <componentName>-v<RevisionNumber>, for example: comp-v1
func ConstructRevisionName(componentName string, revision int64) string {
	return strings.Join([]string{componentName, fmt.Sprintf("v%d", revision)}, "-")
}

// ExtractComponentName will extract componentName from revisionName
func ExtractComponentName(revisionName string) string {
	splits := strings.Split(revisionName, "-")
	return strings.Join(splits[0:len(splits)-1], "-")
}
