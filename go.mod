module github.com/oam-dev/kubevela

go 1.16

require (
	cuelang.org/go v0.2.2
	github.com/AlecAivazis/survey/v2 v2.1.1
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/Netflix/go-expect v0.0.0-20180615182759-c93bf25de8e8
	github.com/agiledragon/gomonkey/v2 v2.3.0
	github.com/alibabacloud-go/cs-20151215/v2 v2.4.5
	github.com/alibabacloud-go/darabonba-openapi v0.1.4
	github.com/alibabacloud-go/tea v1.1.15
	github.com/aryann/difflib v0.0.0-20210328193216-ff5ff6dc229b
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869
	github.com/briandowns/spinner v1.11.1
	github.com/containerd/containerd v1.4.12
	github.com/coreos/prometheus-operator v0.41.1
	github.com/crossplane/crossplane-runtime v0.14.1-0.20210722005935-0b469fcc77cd
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1
	github.com/emicklei/go-restful-openapi/v2 v2.3.0
	github.com/emicklei/go-restful/v3 v3.0.0-rc2
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/fatih/color v1.12.0
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/gertd/go-pluralize v0.1.7
	github.com/getkin/kin-openapi v0.34.0
	github.com/go-logr/logr v0.4.0
	github.com/go-openapi/spec v0.19.8
	github.com/go-playground/validator/v10 v10.9.0
	github.com/go-resty/resty/v2 v2.7.0
	github.com/google/go-cmp v0.5.6
	github.com/google/go-github/v32 v32.1.0
	github.com/google/uuid v1.1.2
	github.com/gosuri/uilive v0.0.4
	github.com/gosuri/uitable v0.0.4
	github.com/hashicorp/hcl/v2 v2.9.1
	github.com/hinshun/vt10x v0.0.0-20180616224451-1954e6464174
	github.com/imdario/mergo v0.3.12
	github.com/kyokomi/emoji v2.2.4+incompatible
	github.com/mitchellh/hashstructure/v2 v2.0.1
	github.com/oam-dev/cluster-gateway v1.1.6
	github.com/oam-dev/cluster-register v1.0.1
	github.com/oam-dev/terraform-config-inspect v0.0.0-20210418082552-fc72d929aa28
	github.com/oam-dev/terraform-controller v0.2.10
	github.com/olekukonko/tablewriter v0.0.5
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/opencontainers/runc v1.0.0-rc95 // indirect
	github.com/openkruise/kruise-api v0.9.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.9.3
	github.com/wercker/stern v0.0.0-20190705090245-4fa46dd6987f
	github.com/wonderflow/cert-manager-api v1.0.3
	go.mongodb.org/mongo-driver v1.5.1
	go.uber.org/zap v1.18.1
	golang.org/x/oauth2 v0.0.0-20210402161424-2e8d93401602
	golang.org/x/sys v0.0.0-20210927094055-39ccf1dd6fa6 // indirect
	golang.org/x/tools v0.1.6 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	gotest.tools v2.2.0+incompatible
	helm.sh/helm/v3 v3.6.1
	istio.io/api v0.0.0-20210128181506-0c4b8e54850f
	istio.io/client-go v0.0.0-20210128182905-ee2edd059e02
	k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/cli-runtime v0.21.0
	k8s.io/client-go v0.22.1
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.9.0
	k8s.io/kube-aggregator v0.22.1
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
	k8s.io/kubectl v0.21.0
	k8s.io/utils v0.0.0-20210802155522-efc7438f0176
	open-cluster-management.io/api v0.0.0-20210804091127-340467ff6239
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.9.5
	sigs.k8s.io/controller-tools v0.6.2
	sigs.k8s.io/kind v0.9.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/docker/docker => github.com/moby/moby v17.12.0-ce-rc1.0.20200618181300-9dc6525e6118+incompatible
	github.com/wercker/stern => github.com/oam-dev/stern v1.13.2
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client => sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.24
)
