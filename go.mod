module github.com/oam-dev/kubevela

go 1.13

require (
	cuelang.org/go v0.2.2
	github.com/AlecAivazis/survey/v2 v2.1.1
	github.com/Netflix/go-expect v0.0.0-20180615182759-c93bf25de8e8
	github.com/briandowns/spinner v1.11.1
	github.com/bugsnag/bugsnag-go v1.5.3 // indirect
	github.com/bugsnag/panicwrap v1.2.0 // indirect
	github.com/coreos/prometheus-operator v0.41.1
	github.com/crossplane/crossplane-runtime v0.9.0
	github.com/crossplane/oam-kubernetes-runtime v0.3.0-rc1.0.20201019050404-723f8ecf8444
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/fatih/color v1.9.0
	github.com/garyburd/redigo v1.6.2 // indirect
	github.com/gertd/go-pluralize v0.1.7
	github.com/ghodss/yaml v1.0.0
	github.com/gin-contrib/static v0.0.0-20200815103939-31fb0c56a3d1
	github.com/gin-gonic/gin v1.6.3
	github.com/go-logr/logr v0.1.0
	github.com/gofrs/uuid v3.3.0+incompatible // indirect
	github.com/golang/gddo v0.0.0-20190419222130-af0f2af80721
	github.com/google/go-cmp v0.5.2
	github.com/google/go-github/v32 v32.1.0
	github.com/gosuri/uitable v0.0.4
	github.com/hinshun/vt10x v0.0.0-20180616224451-1954e6464174
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/kedacore/keda v1.5.1-0.20201009082004-c498970a120e
	github.com/kyokomi/emoji v2.2.4+incompatible
	github.com/mholt/archiver/v3 v3.3.0
	github.com/oam-dev/trait-injector v0.0.0-20200331033130-0a27b176ffc4
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/openservicemesh/osm v0.3.0
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.1-0.20181028125025-b2ce2384e17b
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/wercker/stern v0.0.0-20190705090245-4fa46dd6987f
	github.com/wonderflow/cert-manager-api v1.0.3
	github.com/yvasiyarov/go-metrics v0.0.0-20150112132944-c25f46c4b940 // indirect
	github.com/yvasiyarov/gorelic v0.0.7 // indirect
	github.com/yvasiyarov/newrelic_platform_go v0.0.0-20160601141957-9c099fbc30e9 // indirect
	go.uber.org/zap v1.15.0
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gotest.tools v2.2.0+incompatible
	helm.sh/helm/v3 v3.3.4
	k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/cli-runtime v0.18.8
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.18.8
	k8s.io/utils v0.0.0-20200603063816-c1c6865ac451
	sigs.k8s.io/controller-runtime v0.6.2
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.2.0+incompatible // https://github.com/kubernetes/client-go/issues/628
	github.com/wercker/stern => github.com/oam-dev/stern v1.13.0-alpha
	// clint-go had a buggy release, https://github.com/kubernetes/client-go/issues/749
	k8s.io/client-go => k8s.io/client-go v0.18.8
)

// fix `make` issue
replace github.com/Sirupsen/logrus v1.7.0 => github.com/sirupsen/logrus v1.7.0

replace (
	// fix build issue https://github.com/docker/distribution/issues/2406
	github.com/docker/distribution => github.com/docker/distribution v0.0.0-20191216044856-a8371794149d
	github.com/docker/docker => github.com/moby/moby v17.12.0-ce-rc1.0.20200618181300-9dc6525e6118+incompatible
	// fix build issue https://github.com/ory/dockertest/issues/208
	golang.org/x/sys => golang.org/x/sys v0.0.0-20200826173525-f9321e4c35a6
)
