package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetType(t *testing.T) {
	svc1 := Service{}
	got := svc1.GetType()
	assert.Equal(t, DefaultWorkloadType, got)

	var workload2 = "W2"
	map2 := map[string]interface{}{
		"type": workload2,
		"cpu":  "0.5",
	}
	svc2 := Service(map2)
	got = svc2.GetType()
	assert.Equal(t, workload2, got)
}
