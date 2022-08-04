package config

import "k8s.io/client-go/rest"

// Config application configs
type Config struct {
	RestConfig *rest.Config
}
