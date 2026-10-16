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
Generate certificates for cluster-gateway TLS or reuse existing secret from cluster.
*/}}
{{- define "kubevela-cluster-gateway.certs" -}}
{{- if not .Values.clusterGatewayCerts -}}
  {{- $secretName := printf "%s-cluster-gateway-tls-v2" (include "kubevela.fullname" .) -}}
  {{- $existingSecret := lookup "v1" "Secret" .Release.Namespace $secretName -}}
  {{- $caCert := "" -}}
  {{- $tlsCert := "" -}}
  {{- $tlsKey := "" -}}
  {{- if $existingSecret -}}
    {{- if $existingSecret.data -}}
      {{- if hasKey $existingSecret.data "tls.crt" -}}
        {{- $tlsCert = index $existingSecret.data "tls.crt" -}}
      {{- end -}}
      {{- if hasKey $existingSecret.data "tls.key" -}}
        {{- $tlsKey = index $existingSecret.data "tls.key" -}}
      {{- end -}}
      {{- if hasKey $existingSecret.data "ca.crt" -}}
        {{- $caCert = index $existingSecret.data "ca.crt" -}}
      {{- else if hasKey $existingSecret.data "ca" -}}
        {{- $caCert = index $existingSecret.data "ca" -}}
      {{- end -}}
    {{- end -}}
  {{- end -}}
  {{- if not $caCert -}}
    {{- $apiSvc := lookup "apiregistration.k8s.io/v1" "APIService" "" "v1alpha1.cluster.core.oam.dev" -}}
    {{- if $apiSvc -}}
      {{- if $apiSvc.spec -}}
        {{- if hasKey $apiSvc.spec "caBundle" -}}
          {{- if and $apiSvc.spec.caBundle (ne $apiSvc.spec.caBundle "Cg==") -}}
            {{- $caCert = $apiSvc.spec.caBundle -}}
          {{- end -}}
        {{- end -}}
      {{- end -}}
    {{- end -}}
  {{- end -}}
  {{- $hasAll := false -}}
  {{- if $caCert -}}
    {{- if $tlsCert -}}
      {{- if $tlsKey -}}
        {{- $hasAll = true -}}
      {{- end -}}
    {{- end -}}
  {{- end -}}
  {{- if not $hasAll -}}
    {{- $ca := genCA (printf "%s-cluster-gateway-ca" (include "kubevela.fullname" .)) 3650 -}}
    {{- $cn := printf "%s-cluster-gateway-service" .Release.Name -}}
    {{- $altNames := list $cn (printf "%s.%s.svc" $cn .Release.Namespace) (printf "%s.%s.svc.cluster.local" $cn .Release.Namespace) -}}
    {{- $cert := genSignedCert $cn nil $altNames 3650 $ca -}}
    {{- $caCert = $ca.Cert | b64enc -}}
    {{- $tlsCert = $cert.Cert | b64enc -}}
    {{- $tlsKey = $cert.Key | b64enc -}}
  {{- end -}}
  {{- $_ := set .Values "clusterGatewayCerts" (dict "caCert" $caCert "tlsCert" $tlsCert "tlsKey" $tlsKey) -}}
{{- end -}}
{{- end -}}
