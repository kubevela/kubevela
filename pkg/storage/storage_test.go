package storage

import (
	"reflect"
	"testing"

	"github.com/oam-dev/kubevela/pkg/storage/driver"
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
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewStorage(tt.args.driverName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewStorage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStorage_Delete(t *testing.T) {
	type fields struct {
		Driver driver.Driver
	}
	type args struct {
		envName string
		appName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Storage{
				Driver: tt.fields.Driver,
			}
			if err := s.Delete(tt.args.envName, tt.args.appName); (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStorage_Get(t *testing.T) {
	type fields struct {
		Driver driver.Driver
	}
	type args struct {
		envName string
		appName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *driver.RespApplication
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Storage{
				Driver: tt.fields.Driver,
			}
			got, err := s.Get(tt.args.envName, tt.args.appName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStorage_List(t *testing.T) {
	type fields struct {
		Driver driver.Driver
	}
	type args struct {
		envName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []*driver.RespApplication
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Storage{
				Driver: tt.fields.Driver,
			}
			got, err := s.List(tt.args.envName)
			if (err != nil) != tt.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("List() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStorage_Save(t *testing.T) {
	type fields struct {
		Driver driver.Driver
	}
	type args struct {
		app     *driver.RespApplication
		envName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Storage{
				Driver: tt.fields.Driver,
			}
			if err := s.Save(tt.args.app, tt.args.envName); (err != nil) != tt.wantErr {
				t.Errorf("Save() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
