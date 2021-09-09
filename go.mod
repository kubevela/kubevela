module github.com/oam-dev/kubevela

go 1.16

require (
	cuelang.org/go v0.2.2
	github.com/AlecAivazis/survey/v2 v2.1.1
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/Netflix/go-expect v0.0.0-20180615182759-c93bf25de8e8
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751
	github.com/aryann/difflib v0.0.0-20210328193216-ff5ff6dc229b
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869
	github.com/briandowns/spinner v1.11.1
	github.com/containerd/containerd v1.4.8
	github.com/coreos/prometheus-operator v0.41.1
	github.com/crossplane/crossplane-runtime v0.14.1-0.20210722005935-0b469fcc77cd
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/fatih/color v1.12.0
	github.com/gertd/go-pluralize v0.1.7
	github.com/getkin/kin-openapi v0.34.0
	github.com/gin-contrib/static v0.0.0-20200815103939-31fb0c56a3d1
	github.com/gin-gonic/gin v1.7.0
	github.com/go-logr/logr v0.4.0
	github.com/go-openapi/jsonreference v0.19.5 // indirect
	github.com/go-openapi/spec v0.19.8 // indirect
	github.com/go-openapi/swag v0.19.11 // indirect
	github.com/google/go-cmp v0.5.6
	github.com/google/go-github/v32 v32.1.0
	github.com/gosuri/uitable v0.0.4
	github.com/hashicorp/hcl/v2 v2.9.1
	github.com/hinshun/vt10x v0.0.0-20180616224451-1954e6464174
	github.com/imdario/mergo v0.3.12
	github.com/klauspost/compress v1.10.5 // indirect
	github.com/kyokomi/emoji v2.2.4+incompatible
	github.com/labstack/echo/v4 v4.5.0
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mholt/archiver/v3 v3.3.0
	github.com/mitchellh/hashstructure/v2 v2.0.1
	github.com/oam-dev/terraform-config-inspect v0.0.0-20210418082552-fc72d929aa28
	github.com/oam-dev/terraform-controller v0.1.18
	github.com/olekukonko/tablewriter v0.0.4
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.14.0
	github.com/openkruise/kruise-api v0.9.0
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.1-0.20181028125025-b2ce2384e17b
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/swaggo/files v0.0.0-20190704085106-630677cd5c14
	github.com/swaggo/gin-swagger v1.3.0
	github.com/swaggo/swag v1.6.7
	github.com/tidwall/gjson v1.6.8
	github.com/wercker/stern v0.0.0-20190705090245-4fa46dd6987f
	github.com/wonderflow/cert-manager-api v1.0.3
	go.uber.org/zap v1.18.1
	golang.org/x/oauth2 v0.0.0-20210402161424-2e8d93401602
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	gotest.tools v2.2.0+incompatible
	helm.sh/helm/v3 v3.6.1
	istio.io/api v0.0.0-20210128181506-0c4b8e54850f
	istio.io/client-go v0.0.0-20210128182905-ee2edd059e02
	k8s.io/api v0.21.3
	k8s.io/apiextensions-apiserver v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/cli-runtime v0.21.0
	k8s.io/client-go v0.21.3
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.8.0
	k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
	k8s.io/kubectl v0.21.0
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	open-cluster-management.io/api v0.0.0-20210804091127-340467ff6239
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.9.5
	sigs.k8s.io/controller-tools v0.6.2
	sigs.k8s.io/kind v0.9.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/docker/docker => github.com/moby/moby v17.12.0-ce-rc1.0.20200618181300-9dc6525e6118+incompatible
	github.com/wercker/stern => github.com/oam-dev/stern v1.13.0-alpha
)
