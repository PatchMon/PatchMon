{{/*
Expand the name of the chart.
*/}}
{{- define "patchmon.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "patchmon.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "patchmon.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "patchmon.labels" -}}
helm.sh/chart: {{ include "patchmon.chart" . }}
{{ include "patchmon.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "patchmon.selectorLabels" -}}
app.kubernetes.io/name: {{ include "patchmon.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component labels for database
*/}}
{{- define "patchmon.database.labels" -}}
{{ include "patchmon.labels" . }}
app.kubernetes.io/component: database
{{- end }}

{{/*
Selector labels for database
*/}}
{{- define "patchmon.database.selectorLabels" -}}
{{ include "patchmon.selectorLabels" . }}
app.kubernetes.io/component: database
app: {{ include "patchmon.fullname" . }}-database
{{- end }}

{{/*
Component labels for redis
*/}}
{{- define "patchmon.redis.labels" -}}
{{ include "patchmon.labels" . }}
app.kubernetes.io/component: redis
{{- end }}

{{/*
Selector labels for redis
*/}}
{{- define "patchmon.redis.selectorLabels" -}}
{{ include "patchmon.selectorLabels" . }}
app.kubernetes.io/component: redis
app: {{ include "patchmon.fullname" . }}-redis
{{- end }}

{{/*
Component labels for server
*/}}
{{- define "patchmon.server.labels" -}}
{{ include "patchmon.labels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Selector labels for server
*/}}
{{- define "patchmon.server.selectorLabels" -}}
{{ include "patchmon.selectorLabels" . }}
app.kubernetes.io/component: server
app: {{ include "patchmon.fullname" . }}-server
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "patchmon.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "patchmon.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Get the proper image registry
*/}}
{{- define "patchmon.imageRegistry" -}}
{{- if .global.imageRegistry -}}
{{- .global.imageRegistry -}}
{{- else if .registry -}}
{{- .registry -}}
{{- end -}}
{{- end -}}

{{/*
Return the proper database image name
*/}}
{{- define "patchmon.database.image" -}}
{{- $registry := include "patchmon.imageRegistry" (dict "registry" .Values.database.image.registry "global" .Values.global) -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry .Values.database.image.repository .Values.database.image.tag -}}
{{- else -}}
{{- printf "%s:%s" .Values.database.image.repository .Values.database.image.tag -}}
{{- end -}}
{{- end -}}

{{/*
Return the proper redis image name
*/}}
{{- define "patchmon.redis.image" -}}
{{- $registry := include "patchmon.imageRegistry" (dict "registry" .Values.redis.image.registry "global" .Values.global) -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry .Values.redis.image.repository .Values.redis.image.tag -}}
{{- else -}}
{{- printf "%s:%s" .Values.redis.image.repository .Values.redis.image.tag -}}
{{- end -}}
{{- end -}}

{{/*
Return the proper server image name
*/}}
{{- define "patchmon.server.image" -}}
{{- $registry := include "patchmon.imageRegistry" (dict "registry" .Values.server.image.registry "global" .Values.global) -}}
{{- $tag := .Values.server.image.tag -}}
{{- if .Values.global.imageTag -}}
{{- $tag = .Values.global.imageTag -}}
{{- end -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry .Values.server.image.repository $tag -}}
{{- else -}}
{{- printf "%s:%s" .Values.server.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{/*
Return the proper guacd image name
*/}}
{{- define "patchmon.guacd.image" -}}
{{- $registry := include "patchmon.imageRegistry" (dict "registry" .Values.guacd.image.registry "global" .Values.global) -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry .Values.guacd.image.repository .Values.guacd.image.tag -}}
{{- else -}}
{{- printf "%s:%s" .Values.guacd.image.repository .Values.guacd.image.tag -}}
{{- end -}}
{{- end -}}

{{/*
Component labels for guacd
*/}}
{{- define "patchmon.guacd.labels" -}}
{{ include "patchmon.labels" . }}
app.kubernetes.io/component: guacd
{{- end }}

{{/*
Selector labels for guacd
*/}}
{{- define "patchmon.guacd.selectorLabels" -}}
{{ include "patchmon.selectorLabels" . }}
app.kubernetes.io/component: guacd
app: {{ include "patchmon.fullname" . }}-guacd
{{- end }}

{{/*
Return the proper storage class
*/}}
{{- define "patchmon.storageClass" -}}
{{- $storageClass := .local -}}
{{- if .global -}}
{{- if .global.storageClass -}}
{{- $storageClass = .global.storageClass -}}
{{- end -}}
{{- end -}}
{{- if $storageClass -}}
{{- printf "%s" $storageClass -}}
{{- end -}}
{{- end -}}

{{/*
Return the secret name for database password
*/}}
{{- define "patchmon.database.secretName" -}}
{{- if .Values.database.auth.existingSecret -}}
{{- .Values.database.auth.existingSecret -}}
{{- else -}}
{{- include "patchmon.fullname" . -}}-secrets
{{- end -}}
{{- end -}}

{{/*
Return the secret name for redis password
*/}}
{{- define "patchmon.redis.secretName" -}}
{{- if .Values.redis.auth.existingSecret -}}
{{- .Values.redis.auth.existingSecret -}}
{{- else -}}
{{- include "patchmon.fullname" . -}}-secrets
{{- end -}}
{{- end -}}

{{/*
Return the secret name for server JWT/AI secrets
*/}}
{{- define "patchmon.server.secretName" -}}
{{- if .Values.server.existingSecret -}}
{{- .Values.server.existingSecret -}}
{{- else -}}
{{- include "patchmon.fullname" . -}}-secrets
{{- end -}}
{{- end -}}

{{/*
Return the configmap name
*/}}
{{- define "patchmon.configMapName" -}}
{{- include "patchmon.fullname" . -}}-config
{{- end -}}

{{/*
Return the cluster-internal FQDN for the database service
*/}}
{{- define "patchmon.database.fqdn" -}}
{{- printf "%s-database.%s.svc.cluster.local" (include "patchmon.fullname" .) .Release.Namespace -}}
{{- end -}}

{{/*
Return the cluster-internal FQDN for the redis service
*/}}
{{- define "patchmon.redis.fqdn" -}}
{{- printf "%s-redis.%s.svc.cluster.local" (include "patchmon.fullname" .) .Release.Namespace -}}
{{- end -}}

{{/*
Return the cluster-internal FQDN for the guacd service
*/}}
{{- define "patchmon.guacd.fqdn" -}}
{{- printf "%s-guacd.%s.svc.cluster.local" (include "patchmon.fullname" .) .Release.Namespace -}}
{{- end -}}

{{/*
Common annotations
*/}}
{{- define "patchmon.annotations" -}}
{{- with .Values.commonAnnotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Return the proper init container image name (busybox)
*/}}
{{- define "patchmon.initContainer.image" -}}
{{- $registry := .Values.global.imageRegistry -}}
{{- if $registry -}}
{{- printf "%s/busybox:latest" $registry -}}
{{- else -}}
docker.io/busybox:latest
{{- end -}}
{{- end -}}

{{/*
Return the OIDC redirect URI based on ingress configuration
*/}}
{{- define "patchmon.oidc.redirectUri" -}}
{{- $host := .Values.server.env.serverHost -}}
{{- $protocol := .Values.server.env.serverProtocol | default "http" -}}
{{- $port := .Values.server.env.serverPort | default "80" -}}
{{- if or (eq $port "80") (eq $port "443") -}}
{{- printf "%s://%s/api/v1/auth/oidc/callback" $protocol $host -}}
{{- else -}}
{{- printf "%s://%s:%s/api/v1/auth/oidc/callback" $protocol $host $port -}}
{{- end -}}
{{- end -}}

{{/*
Return the OIDC post logout URI based on ingress configuration
*/}}
{{- define "patchmon.oidc.postLogoutUri" -}}
{{- $host := .Values.server.env.serverHost -}}
{{- $protocol := .Values.server.env.serverProtocol | default "http" -}}
{{- $port := .Values.server.env.serverPort | default "80" -}}
{{- if or (eq $port "80") (eq $port "443") -}}
{{- printf "%s://%s" $protocol $host -}}
{{- else -}}
{{- printf "%s://%s:%s" $protocol $host $port -}}
{{- end -}}
{{- end -}}
