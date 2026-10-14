{{- define "shorty.labels" -}}
app.kubernetes.io/part-of: shorty
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "shorty.hasMonitoring" -}}
{{- if .Capabilities.APIVersions.Has "monitoring.coreos.com/v1/PodMonitor" -}}true{{- end -}}
{{- end -}}
