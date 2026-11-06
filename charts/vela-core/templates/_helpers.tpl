{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "kubevela.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kubevela.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kubevela.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "kubevela.labels" -}}
helm.sh/chart: {{ include "kubevela.chart" . }}
{{ include "kubevela.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "kubevela.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubevela.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "kubevela-cluster-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubevela.name" . }}-cluster-gateway
app.kubernetes.io/instance: {{ .Release.Name }}-cluster-gateway
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "kubevela.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "kubevela.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
systemDefinitionNamespace value defaulter
*/}}
{{- define "systemDefinitionNamespace" -}}
{{- if .Values.systemDefinitionNamespace -}}
    {{ .Values.systemDefinitionNamespace }}
{{- else -}}
    {{ .Release.Namespace }}
{{- end -}}
{{- end -}}

{{/*
definitionRestrictions renders spec.restrictions for one builtin definition.

The chart regenerates these definitions on every upgrade, so a restriction on one
is declared in values and stamped on at render time rather than edited in the
cluster.

Takes a dict of "name" (the definition) and "root" (the chart context), and emits
nothing when no restriction applies.
*/}}
{{- define "definitionRestrictions" -}}
{{- $restrictions := dict -}}
{{- with .root.Values.definitionRestrictions -}}
{{-   with .default -}}
{{-     $restrictions = . -}}
{{-   end -}}
{{-   with .overrides -}}
{{-     if hasKey . $.name -}}
{{-       $restrictions = index . $.name -}}
{{-     end -}}
{{-   end -}}
{{- end -}}
{{- if and (hasKey $restrictions "quota") (not (has .kind (list "ComponentDefinition" "TraitDefinition"))) -}}
{{-   fail (printf "definitionRestrictions would put a quota on %s %q, and only components and traits are counted. A quota under `default` reaches every builtin; under `overrides` it has to name a ComponentDefinition or TraitDefinition." .kind .name) -}}
{{- end -}}
{{- if $restrictions }}
{{ printf "restrictions:\n%s" (toYaml $restrictions | indent 2) | indent 2 }}
{{- end -}}
{{- end -}}

{{/*
builtinDefinitions indexes the files under definitions/ by kind, as YAML mapping
each kind to its "resource" (plural, for kubectl) and the "names" of its
definitions.

The definitions are applied by a post-install/post-upgrade Job rather than as
part of the release, so the webhook that validates them is the one shipped in
this release, not the pod being replaced.
*/}}
{{- define "kubevela.builtinDefinitions" -}}
{{- $index := dict -}}
{{- range $path, $_ := .Files.Glob "definitions/*.yaml" -}}
{{-   $def := tpl ($.Files.Get $path) $ | fromYaml -}}
{{-   $kind := $def.kind -}}
{{-   $entry := get $index $kind | default (dict "resource" (printf "%ss.core.oam.dev" (lower $kind)) "names" list) -}}
{{-   $_ := set $entry "names" (append $entry.names $def.metadata.name) -}}
{{-   $_ := set $index $kind $entry -}}
{{- end -}}
{{- toYaml $index -}}
{{- end -}}

{{/*
builtinDefinitionsConfigMap names the ConfigMap holding the builtin definitions
of one kind. Takes a dict of "kind" and "root" (the chart context).
*/}}
{{- define "kubevela.builtinDefinitionsConfigMap" -}}
{{- printf "%s-builtin-%ss" (include "kubevela.fullname" .root) (lower .kind) -}}
{{- end -}}

{{/*
Returns "true" when vela-core manages the admission webhook serving cert
itself (--webhook-tls-secret). The alternatives are cert-manager and a Secret
that the user supplies.
*/}}
{{- define "kubevela.webhookRuntimeCerts" -}}
{{- if and .Values.admissionWebhooks.enabled .Values.admissionWebhooks.patch.enabled (not .Values.admissionWebhooks.certManager.enabled) -}}true{{- end -}}
{{- end -}}

{{/*
Returns "true" when the chart manages the cluster-gateway APIService: the
APIService does not exist yet, or a Helm release created it. An APIService
without Helm annotations belongs to an external controller, and the chart does
not touch it.
*/}}
{{- define "kubevela.clusterGatewayAPIServiceManaged" -}}
{{- $apiSvc := lookup "apiregistration.k8s.io/v1" "APIService" "" "v1alpha1.cluster.core.oam.dev" -}}
{{- if or (not $apiSvc) (dig "metadata" "annotations" "meta.helm.sh/release-name" "" $apiSvc) -}}true{{- end -}}
{{- end -}}

{{/*
Returns "true" when vela-core manages the cluster-gateway serving cert itself
(--cluster-gateway-tls-secret). The alternative is cert-manager.
*/}}
{{- define "kubevela.clusterGatewayRuntimeCerts" -}}
{{- if and .Values.multicluster.enabled .Values.multicluster.clusterGateway.secureTLS.enabled (not .Values.multicluster.clusterGateway.secureTLS.certManager.enabled) -}}true{{- end -}}
{{- end -}}

{{/*
Returns the webhooks of an existing webhook configuration as JSON, keyed by
webhook name. Takes a dict of "kind" and "name" (the configuration).
*/}}
{{- define "kubevela.existingWebhooks" -}}
{{- $hooks := dict -}}
{{- $existing := lookup "admissionregistration.k8s.io/v1" .kind "" .name -}}
{{- if $existing -}}
{{- range $existing.webhooks -}}{{- $_ := set $hooks .name . -}}{{- end -}}
{{- end -}}
{{- toJson $hooks -}}
{{- end -}}

{{/*
Renders the caBundle line of one webhook. Takes a dict of "root", "hooks"
(from kubevela.existingWebhooks) and "name" (the webhook). It keeps a valid
caBundle that is already in the cluster, so that an upgrade does not break
admission. When vela-core manages the cert, it renders no placeholder, because
vela-core sets the caBundle at startup.
*/}}
{{- define "kubevela.webhookCABundle" -}}
{{- $ca := dig .name "clientConfig" "caBundle" "" .hooks -}}
{{- if include "kubevela.webhookRuntimeCerts" .root -}}
{{- if and $ca (ne $ca "Cg==") }}caBundle: {{ $ca }}{{ end -}}
{{- else -}}
caBundle: {{ default "Cg==" $ca }}
{{- end -}}
{{- end -}}

{{/*
Renders the failurePolicy line of one webhook. Takes a dict of "root", "hooks",
"name" and "policy". "policy" applies when the chart does not patch the
webhooks. With patching on, webhooks start as Ignore until the caBundle is set.
When vela-core manages the cert, vela-core then sets
admissionWebhooks.failurePolicy. Thus the chart keeps the live policy on
upgrade.
*/}}
{{- define "kubevela.webhookFailurePolicy" -}}
{{- $live := dig .name "failurePolicy" "" .hooks -}}
{{- if and (include "kubevela.webhookRuntimeCerts" .root) $live -}}
failurePolicy: {{ $live }}
{{- else if .root.Values.admissionWebhooks.patch.enabled -}}
failurePolicy: Ignore
{{- else -}}
failurePolicy: {{ .policy }}
{{- end -}}
{{- end -}}
