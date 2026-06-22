{{/* Name of the Ceph object bucket backing Mimir's block storage. Must match
     mimir.global.extraEnvFrom's configMapRef/secretRef names in values.yaml,
     which can't reference this helper since they're consumed verbatim by the
     mimir subchart (not passed through tpl). */}}
{{- define "monitoring-server.metricsBucket" -}}
{{- printf "%s-metrics" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
