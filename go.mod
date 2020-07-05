module RudrX

go 1.13

require (
	github.com/crossplane/oam-kubernetes-runtime v0.0.3
	github.com/mitchellh/go-homedir v1.1.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/viper v1.4.0
	github.com/zzxwill/RudrX v0.0.1
	k8s.io/apimachinery v0.18.2
	sigs.k8s.io/yaml v1.2.0
)

replace github.com/zzxwill/RudrX v0.0.1 => ./
