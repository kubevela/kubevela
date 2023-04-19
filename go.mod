module github.com/oam-dev/kubevela

go 1.19

require (
	cuelang.org/go v0.5.0
	github.com/AlecAivazis/survey/v2 v2.1.1
	github.com/FogDong/uitable v0.0.5
	github.com/Masterminds/semver/v3 v3.2.1
	github.com/Netflix/go-expect v0.0.0-20180615182759-c93bf25de8e8
	github.com/alibabacloud-go/cs-20151215/v3 v3.0.32
	github.com/alibabacloud-go/darabonba-openapi/v2 v2.0.4
	github.com/alibabacloud-go/tea v1.1.20
	github.com/aryann/difflib v0.0.0-20210328193216-ff5ff6dc229b
	github.com/bluele/gcache v0.0.2
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869
	github.com/briandowns/spinner v1.11.1
	github.com/chartmuseum/helm-push v0.10.3
	github.com/cloudtty/cloudtty v0.2.0
	github.com/containerd/containerd v1.6.18
	github.com/coreos/prometheus-operator v0.41.1
	github.com/crossplane/crossplane-runtime v0.19.2
	github.com/cue-exp/kubevelafix v0.0.0-20220922150317-aead819d979d
	github.com/dave/jennifer v1.6.1
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.8.0
	github.com/emicklei/go-restful/v3 v3.10.2 // indirect
	github.com/ettle/strcase v0.1.1
	github.com/evanphx/json-patch v5.6.0+incompatible
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/fatih/camelcase v1.0.0 // indirect
	github.com/fatih/color v1.13.0
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/fluxcd/helm-controller/api v0.32.1
	github.com/fluxcd/source-controller/api v0.24.4
	github.com/form3tech-oss/jwt-go v3.2.3+incompatible
	github.com/gdamore/tcell/v2 v2.6.0
	github.com/gertd/go-pluralize v0.2.1
	github.com/getkin/kin-openapi v0.94.0
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-git/go-git/v5 v5.6.1
	github.com/go-logr/logr v1.2.3
	github.com/go-resty/resty/v2 v2.7.0
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.5.9
	github.com/google/go-containerregistry v0.13.0
	github.com/google/go-github/v32 v32.1.0
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/gosuri/uitable v0.0.4
	github.com/hashicorp/go-version v1.6.0
	github.com/hashicorp/hcl/v2 v2.16.2
	github.com/hinshun/vt10x v0.0.0-20180616224451-1954e6464174
	github.com/imdario/mergo v0.3.13
	github.com/kubevela/pkg v1.8.1-0.20230410075324-9f0ba3b09495
	github.com/kubevela/workflow v0.5.1-0.20230412142834-be9e5a10baf0
	github.com/kyokomi/emoji v2.2.4+incompatible
	github.com/magiconair/properties v1.8.7
	github.com/mitchellh/hashstructure/v2 v2.0.2
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/nacos-group/nacos-sdk-go/v2 v2.1.0
	github.com/oam-dev/cluster-gateway v1.9.0-alpha.1.0.20230412030221-d9666385607f
	github.com/oam-dev/cluster-register v1.0.4-0.20220928064144-5f76a9d7ca8c
	github.com/oam-dev/terraform-config-inspect v0.0.0-20210418082552-fc72d929aa28
	github.com/oam-dev/terraform-controller v0.7.10
	github.com/olekukonko/tablewriter v0.0.5
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/ginkgo/v2 v2.9.2
	github.com/onsi/gomega v1.27.5
	github.com/openkruise/kruise-api v1.3.0
	github.com/openkruise/rollouts v0.1.1-0.20220622054609-149e5a48da5e
	github.com/pelletier/go-toml v1.9.5
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/client_model v0.3.0
	github.com/rivo/tview v0.0.0-20221128165837-db36428c92d9
	github.com/rogpeppe/go-internal v1.9.0
	github.com/sirupsen/logrus v1.9.0
	github.com/spf13/cobra v1.6.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.2
	github.com/tidwall/gjson v1.14.4
	github.com/wercker/stern v0.0.0-20190705090245-4fa46dd6987f
	github.com/wonderflow/cert-manager-api v1.0.4-0.20210304051430-e08aa76f6c5f
	github.com/xanzy/go-gitlab v0.80.0
	github.com/xlab/treeprint v1.2.0
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/crypto v0.6.0
	golang.org/x/oauth2 v0.6.0
	golang.org/x/term v0.6.0
	golang.org/x/text v0.8.0
	golang.org/x/tools v0.7.0
	gomodules.xyz/jsonpatch/v2 v2.2.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools v2.2.0+incompatible
	helm.sh/helm/v3 v3.11.2
	istio.io/client-go v1.13.4
	k8s.io/api v0.26.3
	k8s.io/apiextensions-apiserver v0.26.3
	k8s.io/apimachinery v0.26.3
	k8s.io/apiserver v0.26.3
	k8s.io/cli-runtime v0.26.3
	k8s.io/client-go v0.26.3
	k8s.io/component-base v0.26.3
	k8s.io/helm v2.17.0+incompatible
	k8s.io/klog/v2 v2.90.1
	k8s.io/kube-aggregator v0.26.3
	k8s.io/kubectl v0.26.3
	k8s.io/metrics v0.26.3
	k8s.io/utils v0.0.0-20221128185143-99ec85e7a448
	open-cluster-management.io/api v0.10.0
	sigs.k8s.io/controller-runtime v0.14.6
	sigs.k8s.io/controller-tools v0.11.3
	sigs.k8s.io/gateway-api v0.4.3
	sigs.k8s.io/kind v0.17.0
	sigs.k8s.io/yaml v1.3.0
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible // indirect
	github.com/Masterminds/sprig/v3 v3.2.3 // indirect
	github.com/Masterminds/squirrel v1.5.3 // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20230217124315-7d5c6f04bbb8 // indirect
	github.com/acomagu/bufpipe v1.0.4 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/alessio/shellescape v1.4.1 // indirect
	github.com/alibabacloud-go/alibabacloud-gateway-spi v0.0.4 // indirect
	github.com/alibabacloud-go/debug v0.0.0-20190504072949-9472017b5c68 // indirect
	github.com/alibabacloud-go/endpoint-util v1.1.1 // indirect
	github.com/alibabacloud-go/openapi-util v0.1.0 // indirect
	github.com/alibabacloud-go/tea-utils v1.4.5 // indirect
	github.com/alibabacloud-go/tea-utils/v2 v2.0.1 // indirect
	github.com/alibabacloud-go/tea-xml v1.1.2 // indirect
	github.com/aliyun/alibaba-cloud-sdk-go v1.61.1704 // indirect
	github.com/aliyun/credentials-go v1.1.2 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v1.4.10 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20200428143746-21a406dcc535 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/clbanning/mxj/v2 v2.5.5 // indirect
	github.com/cloudflare/circl v1.1.0 // indirect
	github.com/cockroachdb/apd/v2 v2.0.2 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/creack/pty v1.1.18 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/docker/cli v20.10.21+incompatible // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/docker/docker v20.10.24+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/proto v1.10.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fluxcd/pkg/apis/acl v0.0.3 // indirect
	github.com/fluxcd/pkg/apis/kustomize v1.0.0 // indirect
	github.com/fluxcd/pkg/apis/meta v1.0.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/fvbommel/sortorder v1.0.1 // indirect
	github.com/gdamore/encoding v1.0.0 // indirect
	github.com/go-errors/errors v1.0.1 // indirect
	github.com/go-git/gcfg v1.5.0 // indirect
	github.com/go-git/go-billy/v5 v5.4.1 // indirect
	github.com/go-gorp/gorp/v3 v3.1.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/swag v0.19.15 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gobuffalo/flect v0.3.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.0.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/cel-go v0.12.6 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20210407192527-94a9f03dee38 // indirect
	github.com/google/safetext v0.0.0-20220905092116-b49f7bc46da2 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.7.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jmoiron/sqlx v1.3.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/compress v1.16.3 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/pty v1.1.8 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.10.7 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/term v0.0.0-20221205130635-1aeaba878587 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mpvl/unique v0.0.0-20150818121801-cbe035fff7de // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc2 // indirect
	github.com/openshift/library-go v0.0.0-20230327085348-8477ec72b725 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/protocolbuffers/txtpbfmt v0.0.0-20220428173112-74888fd59c2b // indirect
	github.com/rivo/uniseg v0.4.3 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rubenv/sql-migrate v1.3.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/skeema/knownhosts v1.1.0 // indirect
	github.com/spf13/afero v1.8.2 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tjfoc/gmsm v1.3.2 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/zclconf/go-cty v1.12.1 // indirect
	go.etcd.io/etcd/api/v3 v3.5.5 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.5 // indirect
	go.etcd.io/etcd/client/v3 v3.5.5 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.35.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.35.0 // indirect
	go.opentelemetry.io/otel v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.10.0 // indirect
	go.opentelemetry.io/otel/metric v0.31.0 // indirect
	go.opentelemetry.io/otel/sdk v1.10.0 // indirect
	go.opentelemetry.io/otel/trace v1.10.0 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	golang.org/x/mod v0.9.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20221202195650-67e5cbc046fd // indirect
	google.golang.org/grpc v1.50.1 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	istio.io/api v0.0.0-20220512212136-561ffec82582 // indirect
	istio.io/gogo-genproto v0.0.0-20211208193508-5ab4acc9eb1e // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/kms v0.26.3 // indirect
	k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280 // indirect
	oras.land/oras-go v1.2.2 // indirect
	sigs.k8s.io/apiserver-network-proxy v0.0.30 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.36 // indirect
	sigs.k8s.io/apiserver-runtime v1.1.2-0.20221118041430-0a6394f6dda3 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/kustomize/api v0.12.1 // indirect
	sigs.k8s.io/kustomize/kyaml v0.13.9 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect

)

replace (
	cloud.google.com/go => cloud.google.com/go v0.100.2
	github.com/docker/cli => github.com/docker/cli v20.10.9+incompatible
	github.com/docker/docker => github.com/moby/moby v20.10.20+incompatible
	github.com/wercker/stern => github.com/oam-dev/stern v1.13.2
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client => sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.36
)
