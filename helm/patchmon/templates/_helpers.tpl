{{/*
Expand the name of the chart.
*/}}
{{- define "patchmon.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "patchmon.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "patchmon.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "patchmon.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "patchmon.selectorLabels" -}}
app.kubernetes.io/name: {{ include "patchmon.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "patchmon.secretName" -}}
{{- if .Values.secret.create -}}
{{- printf "%s-secrets" (include "patchmon.fullname" .) -}}
{{- else -}}
{{- required "secret.existingSecretName is required when secret.create=false" .Values.secret.existingSecretName -}}
{{- end -}}
{{- end -}}

{{- define "patchmon.databaseServiceName" -}}
{{- default "database" .Values.database.service.name -}}
{{- end -}}

{{- define "patchmon.redisServiceName" -}}
{{- default "redis" .Values.redis.service.name -}}
{{- end -}}

{{- define "patchmon.guacdServiceName" -}}
{{- default "guacd" .Values.guacd.service.name -}}
{{- end -}}

{{- define "patchmon.databasePvcName" -}}
{{- printf "%s-postgres-data" (include "patchmon.fullname" .) -}}
{{- end -}}

{{- define "patchmon.redisPvcName" -}}
{{- printf "%s-redis-data" (include "patchmon.fullname" .) -}}
{{- end -}}
