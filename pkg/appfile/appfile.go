package appfile

import (
	"io/ioutil"

	"github.com/ghodss/yaml"
)

type AppFile struct {
	Name     string             `json:"name"`
	Version  string             `json:"version"`
	Services map[string]Service `json:"services"`
	Secrets  map[string]string  `json:"secrets"`
}

func Load() (*AppFile, error) {
	b, err := ioutil.ReadFile("./vela.yml")
	if err != nil {
		return nil, err
	}
	af := &AppFile{}
	err = yaml.Unmarshal(b, af)
	if err != nil {
		return nil, err
	}
	return af, nil
}
