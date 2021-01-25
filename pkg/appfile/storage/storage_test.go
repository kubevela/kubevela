package storage

import (
	"os"
	"testing"

	"github.com/oam-dev/kubevela/pkg/appfile/storage/driver"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

func TestGetStorage(t *testing.T) {
	_ = os.Setenv(system.StorageDriverEnv, driver.ConfigMapDriverName)

	store := &Storage{driver.NewConfigMapStorage()}
	tests := []struct {
		name string
		want *Storage
	}{
		{name: "TestGetStorage_ConfigMap", want: store},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetStorage(); got.Name() != tt.want.Name() {
				t.Errorf("GetStorage() = %v, want %v", got, tt.want)
			}
		})
	}
}
