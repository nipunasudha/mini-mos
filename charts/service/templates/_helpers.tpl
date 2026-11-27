{{/* Labels that pick this app's pods. Must never change after first deploy (Deployment selector is immutable). */}}
{{- define "service.selectorLabels" -}}
app.kubernetes.io/name: {{ required "name is required" .Values.name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/* Selector labels plus the ones that may change (version = image tag). */}}
{{- define "service.labels" -}}
{{ include "service.selectorLabels" . }}
app.kubernetes.io/version: {{ required "image.tag is required" .Values.image.tag | quote }}
app.kubernetes.io/part-of: mini-mos
{{- end }}

{{/* Same image for the app and the migrate init container; fails the render if repository or tag is missing. */}}
{{- define "service.image" -}}
{{ required "image.repository is required" .Values.image.repository }}:{{ required "image.tag is required" .Values.image.tag }}
{{- end }}

{{/* Only mount the API token when a GCP service account is bound; nothing else in the pod talks to the API server. */}}
{{- define "service.automountToken" -}}
{{ if .Values.serviceAccount.gcpServiceAccount }}true{{ else }}false{{ end }}
{{- end }}

{{/* Shared by the app and init containers, so they cannot drift apart. */}}
{{- define "service.containerSecurityContext" -}}
runAsNonRoot: true
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: ["ALL"]
seccompProfile:
  type: RuntimeDefault
{{- end }}

{{/* env: map becomes a list, envFromSecrets become secretKeyRefs. Emits nothing when both are empty. */}}
{{- define "service.env" -}}
{{- if or .Values.env .Values.envFromSecrets -}}
env:
  {{- range $k, $v := .Values.env }}
  - name: {{ $k }}
    value: {{ $v | quote }}
  {{- end }}
  {{- range .Values.envFromSecrets }}
  - name: {{ .name }}
    valueFrom:
      secretKeyRef:
        name: {{ .secret }}
        key: {{ .key }}
  {{- end }}
{{- end }}
{{- end }}
