{{/* Name of the Alloy OTEL gateway (matches the aliased "otel-gateway" subchart's fullname). */}}
{{- define "monitoring-server.gateway" -}}
{{- printf "%s-otel-gateway" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/* Hostname for the public OTLP ingest endpoint. */}}
{{- define "monitoring-server.gateway.host" -}}
{{- printf "%s.%s" .Values.route.subdomain .Values.platform.domain }}
{{- end }}

{{/* Selector-style labels for the gateway routing resources. */}}
{{- define "monitoring-server.gateway.selectorLabels" -}}
app.kubernetes.io/name: "alloy"
app.kubernetes.io/instance: {{ .Release.Name | quote }}
app.kubernetes.io/component: "otel-gateway"
{{- end }}

{{/* Common labels for the gateway routing resources. */}}
{{- define "monitoring-server.gateway.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" | quote }}
{{ include "monitoring-server.gateway.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service | quote }}
{{- end }}

{{/* Name of the Ceph object bucket backing Loki's log storage. Must match
     loki.singleBinary.extraEnvFrom's configMapRef/secretRef names in
     values.yaml, which can't reference this helper since they're consumed
     verbatim by the loki subchart (not passed through tpl). */}}
{{- define "monitoring-server.logsBucket" -}}
{{- printf "%s-logs" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
