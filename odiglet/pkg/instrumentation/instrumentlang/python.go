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
	envOtelTracesExporter              = "OTEL_TRACES_EXPORTER"
	envOtelMetricsExporter             = "OTEL_METRICS_EXPORTER"
	envValOtelHttpExporter             = "otlp"
	envLogCorrelation                  = "OTEL_PYTHON_LOG_CORRELATION"
	envPythonPath                      = "PYTHONPATH"
	envOtelExporterOTLPTracesProtocol  = "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"
	envOtelExporterOTLPMetricsProtocol = "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL"
	httpProtobufProtocol               = "http/protobuf"
)

func Python(deviceId string, uniqueDestinationSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {
	otlpEndpoint := fmt.Sprintf("http://%s:%d", env.Current.NodeIP, consts.OTLPHttpPort)
	if len(env.Current.APO_COLLECTOR_HTTP_ENDPOINT) > 0 {
		otlpEndpoint = env.Current.APO_COLLECTOR_HTTP_ENDPOINT
	}
	pythonpathVal, _ := envOverwrite.ValToAppend("PYTHONPATH", common.OtelSdkNativeCommunity)

	return &v1beta1.ContainerAllocateResponse{
		Envs: map[string]string{
			envLogCorrelation:                  "true",
			envPythonPath:                      pythonpathVal,
			"OTEL_EXPORTER_OTLP_ENDPOINT":      otlpEndpoint,
			"OTEL_RESOURCE_ATTRIBUTES":         fmt.Sprintf("service.name=%s,odigos.device=python", deviceId),
			envOtelTracesExporter:              envValOtelHttpExporter,
			envOtelMetricsExporter:             envValOtelHttpExporter,
			envOtelExporterOTLPTracesProtocol:  httpProtobufProtocol,
			envOtelExporterOTLPMetricsProtocol: httpProtobufProtocol,
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
