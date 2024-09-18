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
)

func Java(deviceId string, uniqueDestinationSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {
	otlpEndpoint := fmt.Sprintf("http://%s:%d", env.Current.NodeIP, consts.OTLPPort)
	if len(env.Current.APO_COLLECTOR_GRPC_ENDPOINT) > 0 {
		otlpEndpoint = env.Current.APO_COLLECTOR_GRPC_ENDPOINT
	}

	javaOptsVal, _ := envOverwrite.ValToAppend(javaOptsEnvVar, common.OtelSdkNativeCommunity)
	javaToolOptionsVal, _ := envOverwrite.ValToAppend(javaToolOptionsEnvVar, common.OtelSdkNativeCommunity)

	logsExporter := env.Current.OTEL_LOGS_EXPORTER
	metricsExporter := env.Current.OTEL_METRICS_EXPORTER
	tracesExporter := env.Current.OTEL_TRACES_EXPORTER

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
				ContainerPath: commonMountPath,
				HostPath:      commonMountPath,
				ReadOnly:      true,
			},
		},
	}
}
