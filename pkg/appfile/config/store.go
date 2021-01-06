package config

import "errors"

// Store will get config data
type Store interface {
	GetConfigData(configName, envName string) ([]map[string]string, error)
	Type() string
	Namespace(envName string) (string, error)
}

const TypeFake = "fake"

var _ Store = &Fake{}

type Fake struct {
	Data []map[string]string
}

func (f *Fake) GetConfigData(_ string, _ string) ([]map[string]string, error) {
	return f.Data, nil
}

func (Fake) Type() string {
	return TypeFake
}

func (Fake) Namespace(_ string) (string, error) {
	return "", nil
}

func EncodeConfigFormat(key, value string) map[string]string {
	return map[string]string{
		"name":  key,
		"value": value,
	}
}

func DecodeConfigFormat(data []map[string]string) (map[string]string, error) {
	var res = make(map[string]string)
	for _, d := range data {
		key, ok := d["name"]
		if !ok {
			return nil, errors.New("invalid data format, no 'name' found")
		}
		value, ok := d["value"]
		if !ok {
			return nil, errors.New("invalid data format, no 'value' found")
		}
		res[key] = value
	}
}
