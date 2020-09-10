module github.com/cloud-native-application/rudrx

go 1.13

require (
	cuelang.org/go v0.2.2
	github.com/Azure/go-autorest v12.2.0+incompatible // Don't remove. https://github.com/kubernetes/client-go/issues/628
	github.com/coreos/prometheus-operator v0.41.1
	github.com/crossplane/crossplane-runtime v0.9.0
	github.com/crossplane/oam-kubernetes-runtime v0.1.1-0.20200909070723-78b84f2c4799
	github.com/fatih/color v1.9.0
	github.com/gertd/go-pluralize v0.1.7
	github.com/ghodss/yaml v1.0.0
	github.com/gin-contrib/static v0.0.0-20200815103939-31fb0c56a3d1
	github.com/gin-gonic/gin v1.6.3
	github.com/go-logr/logr v0.1.0
	github.com/google/go-cmp v0.5.2
	github.com/google/go-github/v32 v32.1.0
	github.com/gosuri/uitable v0.0.4
	github.com/oam-dev/trait-injector v0.0.0-20200331033130-0a27b176ffc4
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/pkg/errors v0.9.1
	github.com/rs/xid v1.2.1 // indirect
	github.com/satori/go.uuid v1.2.1-0.20181028125025-b2ce2384e17b
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/wercker/stern v0.0.0-20190705090245-4fa46dd6987f
	go.uber.org/zap v1.13.0
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gotest.tools v2.2.0+incompatible
	helm.sh/helm/v3 v3.2.4
	k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver v0.18.2
	k8s.io/apimachinery v0.18.6
	k8s.io/cli-runtime v0.18.6
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200410145947-bcb3869e6f29 // indirect
	k8s.io/kubectl v0.18.6 // indirect
	k8s.io/utils v0.0.0-20200414100711-2df71ebbae66
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.6.0
)

replace (
	github.com/wercker/stern => github.com/oam-dev/stern v1.13.0-alpha
	// clint-go had a buggy release, https://github.com/kubernetes/client-go/issues/749
	k8s.io/client-go => k8s.io/client-go v0.18.6
)
