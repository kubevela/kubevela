{{ define "kubevela-admission-webhook-patch.Podspec" }}
{{- with .Values.imagePullSecrets }}
imagePullSecrets:
{{- toYaml . | nindent 2 }}
{{- end }}
containers:
  - name: patch
    image: {{ .Values.imageRegistry }}{{ .Values.admissionWebhooks.patch.image.repository }}:{{ .Values.admissionWebhooks.patch.image.tag }}
    imagePullPolicy: {{ .Values.admissionWebhooks.patch.image.pullPolicy }}
    args:
      - patch
      - --webhook-name={{ template "kubevela.fullname" . }}-admission
      - --namespace={{ .Release.Namespace }}
      - --secret-name={{ template "kubevela.fullname" . }}-admission
      - --patch-failure-policy={{ .Values.admissionWebhooks.failurePolicy }}
      - --crds=applications.core.oam.dev
restartPolicy: OnFailure
serviceAccountName: {{ template "kubevela.fullname" . }}-admission
{{- with .Values.admissionWebhooks.patch.affinity }}
affinity:
{{ toYaml . | indent 2 }}
{{- end }}
{{- with .Values.admissionWebhooks.patch.tolerations }}
tolerations:
{{ toYaml . | indent 2 }}
{{- end }}
securityContext:
  runAsGroup: 2000
  runAsNonRoot: true
  runAsUser: 2000
{{ end }}

{{ define "kubevela-admission-webhook-create.podSpec" }}
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
      - --host={{ template "kubevela.name" . }}-webhook,{{ template "kubevela.name" . }}-webhook.{{ .Release.Namespace }}.svc
      - --namespace={{ .Release.Namespace }}
      - --secret-name={{ template "kubevela.fullname" . }}-admission
      - --key-name=tls.key
      - --cert-name=tls.crt
restartPolicy: OnFailure
serviceAccountName: {{ template "kubevela.fullname" . }}-admission
{{- with .Values.admissionWebhooks.patch.nodeSelector }}
nodeSelector:
{{- toYaml . | nindent 2 }}
{{- end }}
{{- with .Values.admissionWebhooks.patch.affinity }}
affinity:
{{ toYaml . | indent 2 }}
{{- end }}
{{- with .Values.admissionWebhooks.patch.tolerations }}
tolerations:
{{ toYaml . | indent 2 }}
{{- end }}
securityContext:
  runAsGroup: 2000
  runAsNonRoot: true
  runAsUser: 2000
{{ end }}