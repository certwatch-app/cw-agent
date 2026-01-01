{{/*
Expand the name of the chart.
*/}}
{{- define "cw-agent-certmanager.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "cw-agent-certmanager.fullname" -}}
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
{{- define "cw-agent-certmanager.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cw-agent-certmanager.labels" -}}
helm.sh/chart: {{ include "cw-agent-certmanager.chart" . }}
{{ include "cw-agent-certmanager.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: certwatch
app.kubernetes.io/component: controller
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cw-agent-certmanager.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cw-agent-certmanager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "cw-agent-certmanager.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "cw-agent-certmanager.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Get the API key secret name - supports local, global, or existing secret
Priority: local existingSecret > local value > global existingSecret > global value
*/}}
{{- define "cw-agent-certmanager.apiKeySecretName" -}}
{{- if .Values.apiKey.existingSecret.name -}}
{{- .Values.apiKey.existingSecret.name -}}
{{- else if .Values.apiKey.value -}}
{{- include "cw-agent-certmanager.fullname" . }}-api-key
{{- else if and .Values.global .Values.global.apiKey -}}
{{- if and .Values.global.apiKey.existingSecret .Values.global.apiKey.existingSecret.name -}}
{{- .Values.global.apiKey.existingSecret.name -}}
{{- else if .Values.global.apiKey.value -}}
{{- include "cw-agent-certmanager.fullname" . }}-api-key
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Get the API key secret key name
*/}}
{{- define "cw-agent-certmanager.apiKeySecretKey" -}}
{{- if .Values.apiKey.existingSecret.name -}}
{{- .Values.apiKey.existingSecret.key | default "api-key" -}}
{{- else if .Values.apiKey.value -}}
api-key
{{- else if and .Values.global .Values.global.apiKey -}}
{{- if and .Values.global.apiKey.existingSecret .Values.global.apiKey.existingSecret.name -}}
{{- .Values.global.apiKey.existingSecret.key | default "api-key" -}}
{{- else -}}
api-key
{{- end -}}
{{- else -}}
api-key
{{- end -}}
{{- end -}}

{{/*
Check if we should create a secret (local value or global value set, and no existing secret)
*/}}
{{- define "cw-agent-certmanager.createSecret" -}}
{{- if .Values.apiKey.existingSecret.name -}}
{{- /* Using existing secret, don't create */ -}}
{{- else if .Values.apiKey.value -}}
true
{{- else if and .Values.global .Values.global.apiKey -}}
{{- if and .Values.global.apiKey.existingSecret .Values.global.apiKey.existingSecret.name -}}
{{- /* Using global existing secret, don't create */ -}}
{{- else if .Values.global.apiKey.value -}}
true
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Get the API key value for secret creation
*/}}
{{- define "cw-agent-certmanager.apiKeyValue" -}}
{{- if .Values.apiKey.value -}}
{{- .Values.apiKey.value -}}
{{- else if and .Values.global .Values.global.apiKey .Values.global.apiKey.value -}}
{{- .Values.global.apiKey.value -}}
{{- end -}}
{{- end -}}

{{/*
Get API endpoint - prefer local, fallback to global
*/}}
{{- define "cw-agent-certmanager.apiEndpoint" -}}
{{- if .Values.api.endpoint -}}
{{- .Values.api.endpoint -}}
{{- else if and .Values.global .Values.global.api .Values.global.api.endpoint -}}
{{- .Values.global.api.endpoint -}}
{{- else -}}
https://api.certwatch.app
{{- end -}}
{{- end -}}

{{/*
Get API timeout - prefer local, fallback to global
*/}}
{{- define "cw-agent-certmanager.apiTimeout" -}}
{{- if .Values.api.timeout -}}
{{- .Values.api.timeout -}}
{{- else if and .Values.global .Values.global.api .Values.global.api.timeout -}}
{{- .Values.global.api.timeout -}}
{{- else -}}
30s
{{- end -}}
{{- end -}}

{{/*
Validate that an API key is configured (either local or global)
*/}}
{{- define "cw-agent-certmanager.validateApiKey" -}}
{{- $hasLocalValue := .Values.apiKey.value }}
{{- $hasLocalSecret := .Values.apiKey.existingSecret.name }}
{{- $hasGlobalValue := false }}
{{- $hasGlobalSecret := false }}
{{- if .Values.global }}
  {{- if .Values.global.apiKey }}
    {{- if .Values.global.apiKey.value }}
      {{- $hasGlobalValue = true }}
    {{- end }}
    {{- if .Values.global.apiKey.existingSecret }}
      {{- if .Values.global.apiKey.existingSecret.name }}
        {{- $hasGlobalSecret = true }}
      {{- end }}
    {{- end }}
  {{- end }}
{{- end }}
{{- if not (or $hasLocalValue $hasLocalSecret $hasGlobalValue $hasGlobalSecret) }}
{{- fail "API key is required. Set apiKey.value, apiKey.existingSecret.name, or use global.apiKey in umbrella chart" }}
{{- end }}
{{- end }}
