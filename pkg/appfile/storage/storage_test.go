package storage

import (
	"reflect"
	"testing"

	"github.com/oam-dev/kubevela/pkg/appfile/storage/driver"
)

func TestNewStorage(t *testing.T) {
	type args struct {
		driverName string
	}
	tests := []struct {
		name string
		args args
		want *Storage
	}{
		{"TestNewStorage_Local1", args{""}, &Storage{driver.NewLocalStorage()}},
		{"TestNewStorage_Local2", args{driver.LocalDriverName}, &Storage{driver.NewLocalStorage()}},
		{"TestNewStorage_ConfigMap", args{driver.ConfigMapDriverName}, &Storage{driver.NewConfigMapStorage()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewStorage(tt.args.driverName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewStorage() = %v, want %v", got, tt.want)
			}
		})
	}
}
