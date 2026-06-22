{{/* Name of the Ceph object bucket backing Loki's log storage. Must match
     loki.singleBinary.extraEnvFrom's configMapRef/secretRef names in
     values.yaml, which can't reference this helper since they're consumed
     verbatim by the loki subchart (not passed through tpl). */}}
{{- define "monitoring-server.logsBucket" -}}
{{- printf "%s-logs" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
