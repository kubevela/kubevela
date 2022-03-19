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
PodSpec for both Deployment and Pod
*/}}
{{- define "kubevela.podSpec" -}}
{{- with .Values.imagePullSecrets -}}
imagePullSecrets:
{{- toYaml . | nindent 6 -}}
{{- end -}}
serviceAccountName: {{ include "kubevela.serviceAccountName" . }}
securityContext:
{{- toYaml .Values.podSecurityContext | nindent 2 }}
containers:
- name: {{ .Release.Name }}
  securityContext:
  {{- toYaml .Values.securityContext | nindent 4 }}
  args:
    - "--metrics-addr=:8080"
    - "--enable-leader-election"
    {{- if ne .Values.logFilePath "" }}
    - "--log-file-path={{ .Values.logFilePath }}"
    - "--log-file-max-size={{ .Values.logFileMaxSize }}"
    {{ end -}}
    {{ if .Values.logDebug }}
    - "--log-debug=true"
    {{ end }}
    {{ if .Values.admissionWebhooks.enabled }}
    - "--use-webhook=true"
    - "--webhook-port={{ .Values.webhookService.port }}"
    - "--webhook-cert-dir={{ .Values.admissionWebhooks.certificate.mountPath }}"
    {{ end }}
    - "--health-addr=:{{ .Values.healthCheck.port }}"
    {{ if ne .Values.disableCaps "" }}
    - "--disable-caps={{ .Values.disableCaps }}"
    {{ end }}
    - "--system-definition-namespace={{ include "systemDefinitionNamespace" . }}"
    - "--application-revision-limit={{ .Values.applicationRevisionLimit }}"
    - "--definition-revision-limit={{ .Values.definitionRevisionLimit }}"
    - "--oam-spec-ver={{ .Values.OAMSpecVer }}"
    {{ if .Values.multicluster.enabled }}
    - "--enable-cluster-gateway"
    {{ end }}
    - "--application-re-sync-period={{ .Values.controllerArgs.reSyncPeriod }}"
    - "--concurrent-reconciles={{ .Values.concurrentReconciles }}"
    - "--kube-api-qps={{ .Values.kubeClient.qps }}"
    - "--kube-api-burst={{ .Values.kubeClient.burst }}"
    - "--max-workflow-wait-backoff-time={{ .Values.workflow.backoff.maxTime.waitState }}"
    - "--max-workflow-failed-backoff-time={{ .Values.workflow.backoff.maxTime.failedState }}"
    - "--max-workflow-step-error-retry-times={{ .Values.workflow.step.errorRetryTimes }}"
  image: {{ .Values.imageRegistry }}{{ .Values.image.repository }}:{{ .Values.image.tag }}
  imagePullPolicy: {{ quote .Values.image.pullPolicy }}
  resources:
  {{- toYaml .Values.resources | nindent 4 -}}
  {{ if .Values.admissionWebhooks.enabled }}
  ports:
    - containerPort: {{ .Values.webhookService.port }}
      name: webhook-server
      protocol: TCP
    - containerPort: {{ .Values.healthCheck.port }}
      name: healthz
      protocol: TCP
  readinessProbe:
    httpGet:
      path: /readyz
      port: healthz
    initialDelaySeconds: 30
    periodSeconds: 5
  livenessProbe:
    httpGet:
      path: /healthz
      port: healthz
    initialDelaySeconds: 90
    periodSeconds: 5
  volumeMounts:
    - mountPath: {{ .Values.admissionWebhooks.certificate.mountPath }}
      name: tls-cert-vol
      readOnly: true
  {{ end }}
{{ if .Values.admissionWebhooks.enabled }}
volumes:
- name: tls-cert-vol
  secret:
    defaultMode: 420
    secretName: {{ template "kubevela.fullname" . }}-admission
{{ end }}
{{- with .Values.nodeSelector }}
nodeSelector:
{{- toYaml . | nindent 6 }}
{{- end }}
{{- with .Values.affinity }}
affinity:
{{- toYaml . | nindent 8 }}
{{- end }}
{{- with .Values.tolerations }}
tolerations:
{{- toYaml . | nindent 8 }}
{{- end }}
{{ end }}

{{/*
Cluster Gateway podSpec for both Deployment and Pod
*/}}
{{- define "kubevela-cluster-gateway.podSpec" -}}
{{- with .Values.imagePullSecrets }}
imagePullSecrets:
{{- toYaml . | nindent 2 }}
{{- end }}
serviceAccountName: {{ include "kubevela.serviceAccountName" . }}
securityContext:
{{- toYaml .Values.podSecurityContext | nindent 2 }}
containers:
- name: {{ include "kubevela.fullname" . }}-cluster-gateway
  securityContext:
  {{- toYaml .Values.securityContext | nindent 6 }}
  args:
    - "apiserver"
    - "--secure-port={{ .Values.multicluster.clusterGateway.port }}"
    - "--secret-namespace={{ .Release.Namespace }}"
    - "--feature-gates=APIPriorityAndFairness=false"
    {{ if .Values.multicluster.clusterGateway.secureTLS.enabled }}
    - "--cert-dir={{ .Values.multicluster.clusterGateway.secureTLS.certPath }}"
    {{ end }}
  image: {{ .Values.imageRegistry }}{{ .Values.multicluster.clusterGateway.image.repository }}:{{ .Values.multicluster.clusterGateway.image.tag }}
  imagePullPolicy: {{ .Values.multicluster.clusterGateway.image.pullPolicy }}
  resources:
  {{- toYaml .Values.multicluster.clusterGateway.resources | nindent 6 }}
  ports:
    - containerPort: {{ .Values.multicluster.clusterGateway.port }}
  {{ if .Values.multicluster.clusterGateway.secureTLS.enabled }}
  volumeMounts:
    - mountPath: {{ .Values.multicluster.clusterGateway.secureTLS.certPath }}
      name: tls-cert-vol
      readOnly: true
  {{- end }}
{{ if .Values.multicluster.clusterGateway.secureTLS.enabled }}
volumes:
- name: tls-cert-vol
  secret:
    defaultMode: 420
    secretName: {{ template "kubevela.fullname" . }}-cluster-gateway-tls
{{ end }}
{{- with .Values.nodeSelector }}
nodeSelector:
{{- toYaml . | nindent 2 }}
{{- end }}
{{- with .Values.affinity }}
affinity:
{{- toYaml . | nindent 2 }}
{{- end }}
{{- with .Values.tolerations }}
tolerations:
{{- toYaml . | nindent 2 }}
{{- end }}
{{ end }}

{{ define "kubevela-cluster-gateway-tls-secret-patch.name-labels" }}
name: {{ template "kubevela.fullname" . }}-cluster-gateway-tls-secret-patch
labels:
  app: {{ template "kubevela.fullname" . }}-cluster-gateway-tls-secret-patch
  {{- include "kubevela.labels" . | nindent 2 }}
{{ end }}
{{ define "kubevela-cluster-gateway-tls-secret-patch.annotations" }}
annotations:
  "helm.sh/hook": post-install,post-upgrade
  "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
{{ end }}

{{/*
Cluster Gateway TLS secret patch podSpec for both Job and Pod
*/}}
{{ define "kubevela-cluster-gateway-tls-secret-patch.podSpec" }}
{{- with .Values.imagePullSecrets }}
imagePullSecrets:
{{- toYaml . | nindent 2 }}
{{- end }}
containers:
- name: patch
  image: {{ .Values.imageRegistry }}{{ .Values.multicluster.clusterGateway.image.repository }}:{{ .Values.multicluster.clusterGateway.image.tag }}
  imagePullPolicy: {{ .Values.multicluster.clusterGateway.image.pullPolicy }}
  command:
    - /patch
  args:
    - --secret-namespace={{ .Release.Namespace }}
    - --secret-name={{ template "kubevela.fullname" . }}-cluster-gateway-tls
restartPolicy: OnFailure
serviceAccountName: {{ include "kubevela.serviceAccountName" . }}
securityContext:
  runAsGroup: 2000
  runAsNonRoot: true
  runAsUser: 2000
{{ end }}


{{ define "kubevela-cluster-gateway-tls-secret-create.name-labels" }}
name: {{ template "kubevela.fullname" . }}-cluster-gateway-tls-secret-create
labels:
  app: {{ template "kubevela.fullname" . }}-cluster-gateway-tls-secret-create
  {{- include "kubevela.labels" . | nindent 2 }}
{{ end }}

{{ define "kubevela-cluster-gateway-tls-secret-create.annotations" }}
annotations:
  "helm.sh/hook": pre-install,pre-upgrade
  "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
{{ end }}

{{/*
Cluster Gateway TLS secret create podSpec for both Job and Pod
*/}}
{{ define "kubevela-cluster-gateway-tls-secret-create.podSpec" }}
{{- with .Values.imagePullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
containers:
  - name: create
    image: {{ .Values.imageRegistry }}{{ .Values.admissionWebhooks.patch.image.repository }}:{{ .Values.admissionWebhooks.patch.image.tag }}
    imagePullPolicy: {{ .Values.admissionWebhooks.patch.image.pullPolicy }}
    args:
      - create
      - --host={{ .Release.Name }}-cluster-gateway-service,{{ .Release.Name }}-cluster-gateway-service.{{ .Release.Namespace }}.svc
      - --namespace={{ .Release.Namespace }}
      - --secret-name={{ template "kubevela.fullname" . }}-cluster-gateway-tls
      - --key-name=apiserver.key
      - --cert-name=apiserver.crt
restartPolicy: OnFailure
serviceAccountName: {{ template "kubevela.fullname" . }}-cluster-gateway-admission
securityContext:
  runAsGroup: 2000
  runAsNonRoot: true
  runAsUser: 2000
{{ end }}
