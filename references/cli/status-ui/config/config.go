package config

import "k8s.io/client-go/rest"

type Config struct {
	RestConfig *rest.Config
}
