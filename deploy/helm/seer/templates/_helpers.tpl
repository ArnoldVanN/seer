{{- define "seer.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "seer.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: seer
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/part-of: seer
{{- end -}}

{{/* Selector labels for one component: api, worker, combined or web. */}}
{{- define "seer.selector" -}}
app.kubernetes.io/name: seer
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "seer.serverImage" -}}
{{ .Values.image.server.repository }}:{{ .Values.image.server.tag | default .Chart.AppVersion }}
{{- end -}}

{{- define "seer.webImage" -}}
{{ .Values.image.web.repository }}:{{ .Values.image.web.tag | default .Chart.AppVersion }}
{{- end -}}

{{- define "seer.secretName" -}}
{{ .Values.secrets.existingSecret | default (printf "%s-secrets" (include "seer.fullname" .)) }}
{{- end -}}

{{- define "seer.dbSecretName" -}}
{{ .Values.database.existingSecret | default (printf "%s-db" (include "seer.fullname" .)) }}
{{- end -}}

{{/* Components that serve the API: api in split mode, combined otherwise. */}}
{{- define "seer.apiComponent" -}}
{{ ternary "api" "combined" (eq .Values.mode "split") }}
{{- end -}}

{{/* Env shared by every seer server container. */}}
{{- define "seer.serverEnv" -}}
- name: SEER_DB_DSN
  valueFrom:
    secretKeyRef:
      name: {{ include "seer.dbSecretName" . }}
      key: {{ .Values.database.existingSecretKey }}
- name: SEER_INTERNAL_JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ include "seer.secretName" . }}
      key: internal-jwt-secret
# Newer releases win config sync during a rolling upgrade.
- name: SEER_CONFIG_GENERATION
  value: {{ .Release.Revision | quote }}
- name: SEER_CONFIG
  value: /config/seer.yaml
{{- end -}}

{{/* A seer server Deployment. Args: root, component, values, command. */}}
{{- define "seer.serverDeployment" -}}
{{- $root := .root -}}
{{- $v := .values -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "seer.fullname" $root }}-{{ .component }}
  labels:
    {{- include "seer.labels" $root | nindent 4 }}
    app.kubernetes.io/component: {{ .component }}
spec:
  {{- if not (and $v.hpa $v.hpa.enabled) }}
  replicas: {{ $v.replicas }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "seer.selector" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "seer.selector" . | nindent 8 }}
        app.kubernetes.io/role: server
      annotations:
        checksum/config: {{ include (print $root.Template.BasePath "/configmap.yaml") $root | sha256sum }}
    spec:
      {{- with $root.Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      securityContext:
        {{- toYaml $root.Values.podSecurityContext | nindent 8 }}
      # Workers deregister on SIGTERM so others take over immediately.
      terminationGracePeriodSeconds: 30
      containers:
        - name: seer
          image: {{ include "seer.serverImage" $root }}
          imagePullPolicy: {{ $root.Values.image.pullPolicy }}
          args: [{{ .command | quote }}]
          env:
            {{- include "seer.serverEnv" $root | nindent 12 }}
          ports:
            {{- if ne .component "worker" }}
            - { name: public, containerPort: 8080 }
            - { name: private, containerPort: 8081 }
            {{- end }}
            - { name: ops, containerPort: 9090 }
          readinessProbe:
            httpGet: { path: /readyz, port: ops }
            periodSeconds: 5
          livenessProbe:
            httpGet: { path: /healthz, port: ops }
            periodSeconds: 10
          resources:
            {{- toYaml $v.resources | nindent 12 }}
          securityContext:
            {{- $sc := deepCopy $root.Values.securityContext }}
            {{- if and (ne .component "api") $root.Values.worker.privilegedIcmp }}
            {{- $_ := set $sc "capabilities" (dict "drop" (list "ALL") "add" (list "NET_RAW")) }}
            {{- end }}
            {{- toYaml $sc | nindent 12 }}
          volumeMounts:
            - { name: config, mountPath: /config, readOnly: true }
      volumes:
        - name: config
          configMap:
            name: {{ include "seer.fullname" $root }}-config
      {{- with $v.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $v.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $v.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- if $root.Values.topologySpread.enabled }}
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              {{- include "seer.selector" . | nindent 14 }}
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              {{- include "seer.selector" . | nindent 14 }}
      {{- end }}
{{- end -}}
