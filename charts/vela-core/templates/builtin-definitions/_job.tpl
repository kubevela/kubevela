{{/*
builtinDefinitionsJobValues returns builtinDefinitions.job as YAML, filled in
with its defaults. An upgrade with --reuse-values (vela install's default)
renders with the previous release's values and none of this chart's defaults,
so a release from before these values existed would otherwise render nothing.
*/}}
{{- define "kubevela.builtinDefinitionsJobValues" -}}
{{- $defaults := dict "image" (dict "repository" "alpine/k8s" "tag" "1.31.13" "pullPolicy" "IfNotPresent") "rolloutTimeoutSeconds" 300 "applyRetries" 6 -}}
{{- $set := index (.Values.builtinDefinitions | default dict) "job" | default dict -}}
{{- toYaml (mergeOverwrite $defaults (deepCopy $set)) -}}
{{- end -}}

{{/*
builtinDefinitionsJob renders a hook Job that runs files/builtin-definitions/<action>.sh.
Takes a dict of "root" (the chart context), "action", "hook" and "weight".
*/}}
{{- define "kubevela.builtinDefinitionsJob" -}}
{{- $root := .root -}}
{{- $job := include "kubevela.builtinDefinitionsJobValues" $root | fromYaml -}}
{{- $index := include "kubevela.builtinDefinitions" $root | fromYaml -}}
{{- $refs := list -}}
{{- range $kind := keys $index | sortAlpha -}}
{{-   $entry := get $index $kind -}}
{{-   range $name := $entry.names -}}
{{-     $refs = append $refs (printf "%s/%s" $entry.resource $name) -}}
{{-   end -}}
{{- end -}}
{{- /* Job names and label values are capped at 63 characters, so the base
     names are cut to leave room for "-builtin-definitions-<action>". */ -}}
{{- $suffix := printf "-builtin-definitions-%s" .action -}}
{{- $jobName := printf "%s%s" (include "kubevela.fullname" $root | trunc (sub 63 (len $suffix) | int) | trimSuffix "-") $suffix -}}
{{- $appLabel := printf "%s%s" (include "kubevela.name" $root | trunc (sub 63 (len $suffix) | int) | trimSuffix "-") $suffix -}}
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ $jobName }}
  namespace: {{ $root.Release.Namespace }}
  annotations:
    "helm.sh/hook": {{ .hook }}
    "helm.sh/hook-weight": {{ .weight | quote }}
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
  labels:
    app: {{ $appLabel }}
    {{- include "kubevela.labels" $root | nindent 4 }}
spec:
  backoffLimit: 1
  template:
    metadata:
      # Not kubevela.labels: its selector labels would make this pod an endpoint
      # of the webhook Service, and the API server would send the definition
      # writes this Job makes to the Job itself.
      labels:
        app: {{ $appLabel }}
    spec:
      {{- with $root.Values.imagePullSecrets }}
      imagePullSecrets:
      {{- toYaml . | nindent 8 }}
      {{- end }}
      restartPolicy: Never
      # The controller's ServiceAccount exists in every phase these Jobs run in:
      # it is part of the release, and a pre-upgrade or pre-delete hook runs
      # before Helm changes or removes it.
      serviceAccountName: {{ include "kubevela.serviceAccountName" $root }}
      containers:
        - name: {{ .action }}
          image: {{ $root.Values.imageRegistry }}{{ $job.image.repository }}:{{ $job.image.tag }}
          imagePullPolicy: {{ $job.image.pullPolicy }}
          command:
            - /bin/sh
            - -c
            - |
              {{- $root.Files.Get (printf "files/builtin-definitions/%s.sh" .action) | nindent 14 }}
          env:
            # kubectl writes its discovery cache under $HOME, and the image's
            # home is not writable by the non-root user below.
            - name: HOME
              value: /tmp
            - name: RELEASE_NAME
              value: {{ $root.Release.Name | quote }}
            - name: DEFINITION_NAMESPACE
              value: {{ include "systemDefinitionNamespace" $root | trim | quote }}
            - name: CONTROLLER_NAMESPACE
              value: {{ $root.Release.Namespace | quote }}
            - name: CONTROLLER_DEPLOYMENT
              value: {{ include "kubevela.fullname" $root | quote }}
            - name: ROLLOUT_TIMEOUT_SECONDS
              value: {{ $job.rolloutTimeoutSeconds | quote }}
            - name: APPLY_RETRIES
              value: {{ $job.applyRetries | quote }}
            - name: BUILTIN_DEFINITIONS
              value: {{ join " " $refs | quote }}
            - name: WEBHOOK_CONFIGURATION
              value: {{ ternary (printf "%s-admission" (include "kubevela.fullname" $root)) "" $root.Values.admissionWebhooks.enabled | quote }}
          volumeMounts:
            - name: definitions
              mountPath: /definitions
              readOnly: true
      volumes:
        # Optional, because the pre-upgrade Job runs before this release's
        # ConfigMaps exist.
        - name: definitions
          projected:
            sources:
              {{- range $kind := keys $index | sortAlpha }}
              - configMap:
                  name: {{ include "kubevela.builtinDefinitionsConfigMap" (dict "kind" $kind "root" $root) }}
                  optional: true
              {{- end }}
      {{- with $root.Values.nodeSelector }}
      nodeSelector:
      {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $root.Values.affinity }}
      affinity:
      {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $root.Values.tolerations }}
      tolerations:
      {{- toYaml . | nindent 8 }}
      {{- end }}
      securityContext:
        runAsGroup: 2000
        runAsNonRoot: true
        runAsUser: 2000
{{- end -}}
