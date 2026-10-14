{{- define "shorty-lib.deployment" -}}
{{- $name := include "shorty-lib.name" . -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ $name }}
  labels:
    {{- include "shorty-lib.labels" . | nindent 4 }}
spec:
  {{- if not .Values.hpa.enabled }}
  replicas: {{ .Values.replicaCount }}
  {{- end }}
  revisionHistoryLimit: 5
  selector:
    matchLabels:
      {{- include "shorty-lib.selectorLabels" . | nindent 6 }}
  strategy:
    type: RollingUpdate
    rollingUpdate:
      # Never drop below the desired count during a rollout.
      maxSurge: {{ .Values.strategy.maxSurge }}
      maxUnavailable: {{ .Values.strategy.maxUnavailable }}
  template:
    metadata:
      labels:
        {{- include "shorty-lib.labels" . | nindent 8 }}
      annotations:
        {{- with .Values.podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
        checksum/env: {{ toJson .Values.env | sha256sum | trunc 16 }}
    spec:
      serviceAccountName: {{ $name }}
      automountServiceAccountToken: false
      enableServiceLinks: false
      terminationGracePeriodSeconds: {{ .Values.shutdown.terminationGracePeriodSeconds }}
      securityContext:
        runAsNonRoot: true
        runAsUser: {{ .Values.securityContext.runAsUser }}
        runAsGroup: {{ .Values.securityContext.runAsUser }}
        fsGroup: {{ .Values.securityContext.runAsUser }}
        seccompProfile:
          type: RuntimeDefault
      {{- if .Values.topologySpread.enabled }}
      topologySpreadConstraints:
        # Spread across zones (agents are labelled zone-a/b/c) ...
        - maxSkew: {{ .Values.topologySpread.maxSkew }}
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: {{ .Values.topologySpread.zoneWhenUnsatisfiable }}
          nodeTaintsPolicy: Honor   # cordoned/drained nodes don't count as a domain
          labelSelector:
            matchLabels:
              {{- include "shorty-lib.selectorLabels" . | nindent 14 }}
          matchLabelKeys: [pod-template-hash]
        # ... and across nodes.
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              {{- include "shorty-lib.selectorLabels" . | nindent 14 }}
          matchLabelKeys: [pod-template-hash]
      {{- end }}
      containers:
        - name: {{ $name }}
          image: {{ include "shorty-lib.image" . }}
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          {{- with .Values.args }}
          args:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          ports:
            - name: http
              containerPort: {{ .Values.ports.http }}
            - name: admin
              containerPort: {{ .Values.ports.admin }}
          env:
            {{- if .Values.goRuntime }}
            # GOMAXPROCS follows the CPU limit automatically (Go >= 1.25);
            # GOMEMLIMIT is derived from the memory limit (see pkg/memlimit).
            - name: MEMORY_LIMIT_BYTES
              valueFrom:
                resourceFieldRef:
                  resource: limits.memory
            - name: GOMEMLIMIT_RATIO
              value: {{ .Values.goRuntime.memLimitRatio | quote }}
            - name: SHUTDOWN_DRAIN_DELAY
              value: {{ .Values.shutdown.drainDelay | quote }}
            - name: SHUTDOWN_TIMEOUT
              value: {{ .Values.shutdown.timeout | quote }}
            - name: HTTP_ADDR
              value: {{ printf ":%v" .Values.ports.http | quote }}
            - name: ADMIN_ADDR
              value: {{ printf ":%v" .Values.ports.admin | quote }}
            {{- end }}
            {{- range $k, $v := .Values.env }}
            - name: {{ $k }}
              value: {{ $v | toString | quote }}
            {{- end }}
            {{- range .Values.secretEnv }}
            - name: {{ .name }}
              valueFrom:
                secretKeyRef:
                  name: {{ .secretName }}
                  key: {{ .key }}
            {{- end }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
          startupProbe:
            httpGet: { path: {{ .Values.probes.startup.path }}, port: admin }
            periodSeconds: {{ .Values.probes.startup.periodSeconds }}
            failureThreshold: {{ .Values.probes.startup.failureThreshold }}
          livenessProbe:
            httpGet: { path: {{ .Values.probes.liveness.path }}, port: admin }
            periodSeconds: {{ .Values.probes.liveness.periodSeconds }}
            timeoutSeconds: 2
            failureThreshold: {{ .Values.probes.liveness.failureThreshold }}
          readinessProbe:
            httpGet: { path: {{ .Values.probes.readiness.path }}, port: admin }
            periodSeconds: {{ .Values.probes.readiness.periodSeconds }}
            timeoutSeconds: 3
            failureThreshold: {{ .Values.probes.readiness.failureThreshold }}
          lifecycle:
            preStop:
              {{- if .Values.shutdown.preStopExec }}
              exec:
                command: {{ toJson .Values.shutdown.preStopExec }}
              {{- else }}
              # Give kube-proxy/Traefik time to drop this endpoint before the
              # process sees SIGTERM (native sleep: distroless has no shell).
              sleep:
                seconds: {{ .Values.shutdown.preStopSleepSeconds }}
              {{- end }}
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: [ALL]
          {{- with .Values.volumeMounts }}
          volumeMounts:
            {{- toYaml . | nindent 12 }}
          {{- end }}
        {{- with .Values.extraContainers }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- with .Values.volumes }}
      volumes:
        {{- toYaml . | nindent 8 }}
      {{- end }}
{{- end -}}
