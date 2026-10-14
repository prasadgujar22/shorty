{{/* Name of the service, e.g. links-svc. */}}
{{- define "shorty-lib.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "shorty-lib.selectorLabels" -}}
app.kubernetes.io/name: {{ include "shorty-lib.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "shorty-lib.labels" -}}
{{ include "shorty-lib.selectorLabels" . }}
app.kubernetes.io/part-of: shorty
app.kubernetes.io/component: {{ .Values.component | default "backend" }}
app.kubernetes.io/version: {{ include "shorty-lib.tag" . | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "shorty-lib.tag" -}}
{{- .Values.image.tag | default .Chart.AppVersion -}}
{{- end -}}

{{/* registry/repository:tag — global.imageRegistry overrides per-chart registry. */}}
{{- define "shorty-lib.image" -}}
{{- $registry := (.Values.global).imageRegistry | default .Values.image.registry -}}
{{- printf "%s/%s:%s" $registry .Values.image.repository (include "shorty-lib.tag" .) -}}
{{- end -}}

{{- define "shorty-lib.hasMonitoring" -}}
{{- if .Capabilities.APIVersions.Has "monitoring.coreos.com/v1/ServiceMonitor" -}}true{{- end -}}
{{- end -}}
