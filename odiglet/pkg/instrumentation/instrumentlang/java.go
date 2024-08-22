package instrumentlang

import (
	"fmt"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/envOverwrite"
	"github.com/odigos-io/odigos/odiglet/pkg/env"
	"github.com/odigos-io/odigos/odiglet/pkg/instrumentation/consts"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	otelResourceAttributesEnvVar  = "OTEL_RESOURCE_ATTRIBUTES"
	otelResourceAttrPattern       = "service.name=%s,odigos.device=java"
	javaToolOptionsEnvVar         = "JAVA_TOOL_OPTIONS"
	javaOptsEnvVar                = "JAVA_OPTS"
	javaOtlpEndpointEnvVar        = "OTEL_EXPORTER_OTLP_ENDPOINT"
	javaOtlpProtocolEnvVar        = "OTEL_EXPORTER_OTLP_PROTOCOL"
	javaOtelLogsExporterEnvVar    = "OTEL_LOGS_EXPORTER"
	javaOtelMetricsExporterEnvVar = "OTEL_METRICS_EXPORTER"
	javaOtelTracesExporterEnvVar  = "OTEL_TRACES_EXPORTER"
	javaOtelTracesSamplerEnvVar   = "OTEL_TRACES_SAMPLER"

	swCollectorBackendServiceEnvVar = "SW_AGENT_COLLECTOR_BACKEND_SERVICES"
	swLOGGING_OUTPUT                = "SW_LOGGING_OUTPUT"
	swLOGGING_DIR                   = "SW_LOGGING_DIR"
	swLOGGING_FILE_NAME             = "SW_LOGGING_FILE_NAME"
	swLOGGING_MAX_FILE_SIZE         = "SW_LOGGING_MAX_FILE_SIZE"
	swLOGGING_MAX_HISTORY_FILES     = "SW_LOGGING_MAX_HISTORY_FILES"
)

func Java(deviceId string, uniqueDestinationSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {

	javaOptsVal, _ := envOverwrite.ValToAppend(javaOptsEnvVar, common.OtelSdkNativeCommunity)
	javaToolOptionsVal, _ := envOverwrite.ValToAppend(javaToolOptionsEnvVar, common.OtelSdkNativeCommunity)

	logsExporter := "none"
	metricsExporter := "none"
	tracesExporter := "none"

	otlpEndpoint := fmt.Sprintf("http://%s:%d", env.Current.NodeIP, consts.OTLPPort)
	if len(env.Current.OTEL_EXPORTER_OTLP_GRPC_ENDPOINT) > 0 {
		otlpEndpoint = env.Current.OTEL_EXPORTER_OTLP_GRPC_ENDPOINT
		tracesExporter = "otlp"
	}

	return &v1beta1.ContainerAllocateResponse{
		Envs: map[string]string{
			otelResourceAttributesEnvVar:  fmt.Sprintf(otelResourceAttrPattern, deviceId),
			javaToolOptionsEnvVar:         javaToolOptionsVal,
			javaOptsEnvVar:                javaOptsVal,
			javaOtlpEndpointEnvVar:        otlpEndpoint,
			javaOtlpProtocolEnvVar:        "grpc",
			javaOtelLogsExporterEnvVar:    logsExporter,
			javaOtelMetricsExporterEnvVar: metricsExporter,
			javaOtelTracesExporterEnvVar:  tracesExporter,
			javaOtelTracesSamplerEnvVar:   "always_on",
		},
		Mounts: []*v1beta1.Mount{
			{
				ContainerPath: "/var/odigos/java",
				HostPath:      "/var/odigos/java",
				ReadOnly:      true,
			},
		},
	}
}

func JavaInSkywalking(deviceId string, uniqueDestinationSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {
	javaOptsVal, _ := envOverwrite.ValToAppend(javaOptsEnvVar, common.SWSdkNativeCommunity)
	javaToolOptionsVal, _ := envOverwrite.ValToAppend(javaToolOptionsEnvVar, common.SWSdkNativeCommunity)

	var envs = map[string]string{
		javaToolOptionsEnvVar: javaToolOptionsVal,
		javaOptsEnvVar:        javaOptsVal,
	}

	if len(env.Current.SW_AGENT_COLLECTOR_BACKEND_SERVICES) > 0 {
		envs[swCollectorBackendServiceEnvVar] = env.Current.SW_AGENT_COLLECTOR_BACKEND_SERVICES
	} else {
		envs[swCollectorBackendServiceEnvVar] = fmt.Sprintf("%s:%d", env.Current.NodeIP, consts.SWAgentPort)
	}

	if len(env.Current.SW_LOGGING_OUTPUT) > 0 {
		envs[swLOGGING_OUTPUT] = env.Current.SW_LOGGING_OUTPUT
		if len(env.Current.SW_LOGGING_DIR) > 0 {
			envs[swLOGGING_DIR] = env.Current.SW_LOGGING_DIR
		}
		if len(env.Current.SW_LOGGING_FILE_NAME) > 0 {
			envs[swLOGGING_FILE_NAME] = env.Current.SW_LOGGING_FILE_NAME
		}
		if len(env.Current.SW_LOGGING_MAX_FILE_SIZE) > 0 {
			envs[swLOGGING_MAX_FILE_SIZE] = env.Current.SW_LOGGING_MAX_FILE_SIZE
		}
		if len(env.Current.SW_LOGGING_MAX_HISTORY_FILES) > 0 {
			envs[swLOGGING_MAX_HISTORY_FILES] = env.Current.SW_LOGGING_MAX_HISTORY_FILES
		}
	} else {
		envs[swLOGGING_OUTPUT] = "CONSOLE"
	}

	return &v1beta1.ContainerAllocateResponse{
		Envs: envs,
		Mounts: []*v1beta1.Mount{
			{
				ContainerPath: "/var/odigos/skywalking",
				HostPath:      "/var/odigos/skywalking",
				ReadOnly:      true,
			},
		},
	}
}
