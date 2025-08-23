{{/* Base chart name */}}
{{- define "codespace-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Base fullname for this release */}}
{{- define "codespace-operator.fullname" -}}
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

{{/* Common labels for all resources */}}
{{- define "codespace-operator.labels" -}}
app.kubernetes.io/name: {{ include "codespace-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{/* Selector labels: pass a stable app name and the root context */}}
{{- define "codespace-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
{{- end -}}

{{/* ServiceAccount name */}}
{{- define "codespace-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "codespace-operator.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/* Workload names */}}
{{- define "codespace-operator.sessionController.name" -}}
{{- include "codespace-operator.fullname" . -}}
{{- end -}}

{{- define "codespace-operator.server.name" -}}
{{- printf "%s-server" (include "codespace-operator.fullname" .) -}}
{{- end -}}

{{/* Services */}}
{{- define "codespace-operator.metrics.serviceName" -}}
{{- $d := printf "%s-metrics-service" (include "codespace-operator.fullname" .) -}}
{{- default (printf "%s-metrics-service" (include "codespace-operator.fullname" .)) .Values.metrics.service.name -}}
{{- end -}}

{{- define "codespace-operator.server.serviceName" -}}
{{- $default := include "codespace-operator.server.name" . -}}
{{- $val := .Values.server.service.name | default $default -}}
{{- tpl $val . -}}
{{- end -}}

{{- define "codespace-operator.server.serviceAccountName" -}}
{{- if .Values.server.serviceAccount.create -}}
{{- default (printf "%s-server" (include "codespace-operator.fullname" .)) .Values.server.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.server.serviceAccount.name -}}
{{- end -}}
{{- end -}}
