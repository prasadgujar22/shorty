{{- define "shorty-lib.service" -}}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "shorty-lib.name" . }}
  labels:
    {{- include "shorty-lib.labels" . | nindent 4 }}
spec:
  type: ClusterIP
  selector:
    {{- include "shorty-lib.selectorLabels" . | nindent 4 }}
  ports:
    - name: http
      port: {{ .Values.service.port }}
      targetPort: http
    - name: admin
      port: {{ .Values.ports.admin }}
      targetPort: admin
    {{- with .Values.service.extraPorts }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
{{- end -}}

{{- define "shorty-lib.serviceaccount" -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "shorty-lib.name" . }}
  labels:
    {{- include "shorty-lib.labels" . | nindent 4 }}
automountServiceAccountToken: false
{{- end -}}

{{- define "shorty-lib.pdb" -}}
{{- if .Values.pdb.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "shorty-lib.name" . }}
  labels:
    {{- include "shorty-lib.labels" . | nindent 4 }}
spec:
  {{- if .Values.pdb.minAvailable }}
  minAvailable: {{ .Values.pdb.minAvailable }}
  {{- else }}
  maxUnavailable: {{ .Values.pdb.maxUnavailable }}
  {{- end }}
  # Evict pods that never became ready, so a broken rollout can't block drains.
  unhealthyPodEvictionPolicy: AlwaysAllow
  selector:
    matchLabels:
      {{- include "shorty-lib.selectorLabels" . | nindent 6 }}
{{- end }}
{{- end -}}

{{- define "shorty-lib.hpa" -}}
{{- if .Values.hpa.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "shorty-lib.name" . }}
  labels:
    {{- include "shorty-lib.labels" . | nindent 4 }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "shorty-lib.name" . }}
  minReplicas: {{ .Values.hpa.minReplicas }}
  maxReplicas: {{ .Values.hpa.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.hpa.cpuUtilization }}
    {{- if .Values.hpa.rps.enabled }}
    # Custom metric served by prometheus-adapter from shorty_http_requests_total.
    - type: Pods
      pods:
        metric:
          name: {{ .Values.hpa.rps.metric }}
        target:
          type: AverageValue
          averageValue: {{ .Values.hpa.rps.averageValue | quote }}
    {{- end }}
  behavior:
    {{- toYaml .Values.hpa.behavior | nindent 4 }}
{{- end }}
{{- end -}}

{{- define "shorty-lib.servicemonitor" -}}
{{- if and .Values.serviceMonitor.enabled (include "shorty-lib.hasMonitoring" .) }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "shorty-lib.name" . }}
  labels:
    {{- include "shorty-lib.labels" . | nindent 4 }}
spec:
  selector:
    matchLabels:
      {{- include "shorty-lib.selectorLabels" . | nindent 6 }}
  endpoints:
    - port: {{ .Values.serviceMonitor.port }}
      path: /metrics
      interval: {{ .Values.serviceMonitor.interval }}
{{- end }}
{{- end -}}

{{/* Everything a service chart needs. */}}
{{- define "shorty-lib.all" -}}
{{ include "shorty-lib.serviceaccount" . }}
---
{{ include "shorty-lib.service" . }}
---
{{ include "shorty-lib.deployment" . }}
{{- with (include "shorty-lib.pdb" .) }}
---
{{ . }}
{{- end }}
{{- with (include "shorty-lib.hpa" .) }}
---
{{ . }}
{{- end }}
{{- with (include "shorty-lib.servicemonitor" .) }}
---
{{ . }}
{{- end }}
{{- end -}}
