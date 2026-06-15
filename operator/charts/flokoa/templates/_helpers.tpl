{{/*
Expand the name of the chart.
*/}}
{{- define "flokoa.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "flokoa.fullname" -}}
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
{{- define "flokoa.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Namespace helper.
*/}}
{{- define "flokoa.namespace" -}}
{{- .Release.Namespace }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "flokoa.labels" -}}
helm.sh/chart: {{ include "flokoa.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ include "flokoa.name" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Controller fullname.
*/}}
{{- define "flokoa.controller.fullname" -}}
{{- printf "%s-controller" (include "flokoa.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Controller selector labels.
*/}}
{{- define "flokoa.controller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "flokoa.name" . }}
app.kubernetes.io/component: controller
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Controller labels.
*/}}
{{- define "flokoa.controller.labels" -}}
{{ include "flokoa.labels" . }}
{{ include "flokoa.controller.selectorLabels" . }}
{{- end }}

{{/*
Controller service account name.
*/}}
{{- define "flokoa.controller.serviceAccountName" -}}
{{- if .Values.controller.serviceAccount.create }}
{{- default (include "flokoa.controller.fullname" .) .Values.controller.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.controller.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Server fullname.
*/}}
{{- define "flokoa.server.fullname" -}}
{{- printf "%s-server" (include "flokoa.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Server service account name.
*/}}
{{- define "flokoa.server.serviceAccountName" -}}
{{- if .Values.server.serviceAccount.create }}
{{- default (include "flokoa.server.fullname" .) .Values.server.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.server.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Server selector labels.
*/}}
{{- define "flokoa.server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "flokoa.name" . }}
app.kubernetes.io/component: server
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Server labels.
*/}}
{{- define "flokoa.server.labels" -}}
{{ include "flokoa.labels" . }}
{{ include "flokoa.server.selectorLabels" . }}
{{- end }}

{{/*
Dex fullname.
*/}}
{{- define "flokoa.dex.fullname" -}}
{{- printf "%s-dex" (include "flokoa.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Dex selector labels.
*/}}
{{- define "flokoa.dex.selectorLabels" -}}
app.kubernetes.io/name: {{ include "flokoa.name" . }}
app.kubernetes.io/component: dex
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Dex labels.
*/}}
{{- define "flokoa.dex.labels" -}}
{{ include "flokoa.labels" . }}
{{ include "flokoa.dex.selectorLabels" . }}
{{- end }}

{{/*
Construct an image reference from registry, repository, and tag.
Usage: {{ include "flokoa.image" (dict "image" .Values.controller.image "appVersion" .Chart.AppVersion) }}
*/}}
{{- define "flokoa.image" -}}
{{- $tag := default .appVersion .image.tag -}}
{{- if .image.registry -}}
{{- printf "%s/%s:%s" .image.registry .image.repository $tag -}}
{{- else -}}
{{- printf "%s:%s" .image.repository $tag -}}
{{- end -}}
{{- end }}

{{/*
Webhook service name.
*/}}
{{- define "flokoa.webhook.serviceName" -}}
{{- printf "%s-webhook" (include "flokoa.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Webhook certificate name (cert-manager Certificate resource).
*/}}
{{- define "flokoa.webhook.certificateName" -}}
{{- printf "%s-webhook-cert" (include "flokoa.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Webhook serving certificate secret name.
*/}}
{{- define "flokoa.webhook.certSecretName" -}}
{{- default (printf "%s-webhook-server-cert" (include "flokoa.fullname" .) | trunc 63 | trimSuffix "-") .Values.webhooks.certSecretName }}
{{- end }}

{{/*
Validate the capabilities.verification block (roadmap 09). Renders nothing on
success; fails the release on contradictory config:
- requireVerified without cosign verification would deny every capability
  attachment (the Verified condition stays Unknown forever).
- cosign enabled needs exactly one mode: keySecretRef, or BOTH keyless fields.
*/}}
{{- define "flokoa.validateCapabilitiesVerification" -}}
{{- $v := .Values.capabilities.verification -}}
{{- if and $v.requireVerified (not $v.cosign.enabled) -}}
{{- fail "capabilities.verification.requireVerified requires capabilities.verification.cosign.enabled: without signature verification the Verified condition stays Unknown and the policy would deny every capability attachment" -}}
{{- end -}}
{{- if $v.cosign.enabled -}}
{{- if and $v.cosign.keySecretRef (or $v.cosign.keyless.issuer $v.cosign.keyless.identityRegexp) -}}
{{- fail "capabilities.verification.cosign: configure either keySecretRef (key-based) or the keyless block, not both" -}}
{{- end -}}
{{- if and (not $v.cosign.keySecretRef) (not (and $v.cosign.keyless.issuer $v.cosign.keyless.identityRegexp)) -}}
{{- fail "capabilities.verification.cosign.enabled requires keySecretRef, or BOTH keyless.issuer and keyless.identityRegexp (identity-free keyless verification is rejected)" -}}
{{- end -}}
{{- end -}}
{{- end }}
