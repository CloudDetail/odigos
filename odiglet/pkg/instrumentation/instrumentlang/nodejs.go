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
	nodeEnvEndpoint       = "OTEL_EXPORTER_OTLP_ENDPOINT"
	nodeEnvServiceName    = "OTEL_SERVICE_NAME"
	nodeEnvNodeOptions    = "NODE_OPTIONS"
	nodeOdigosOpampServer = "ODIGOS_OPAMP_SERVER_HOST"
	nodeOdigosDeviceId    = "ODIGOS_INSTRUMENTATION_DEVICE_ID"
)

func NodeJS(deviceId string, uniqueDestinationSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {
	otlpEndpoint := fmt.Sprintf("http://%s:%d", env.Current.NodeIP, consts.OTLPPort)
	if len(env.Current.APO_COLLECTOR_GRPC_ENDPOINT) > 0 {
		otlpEndpoint = env.Current.APO_COLLECTOR_GRPC_ENDPOINT
	}
	nodeOptionsVal, _ := envOverwrite.ValToAppend(nodeEnvNodeOptions, common.OtelSdkNativeCommunity)
	opampServerHost := fmt.Sprintf("%s:%d", env.Current.NodeIP, consts.OpAMPPort)
	if len(env.Current.OTEL_EXPORTER_OPAMP_ENDPOINT) > 0 {
		opampServerHost = env.Current.OTEL_EXPORTER_OPAMP_ENDPOINT
	}

	return &v1beta1.ContainerAllocateResponse{
		Envs: map[string]string{
			nodeEnvEndpoint:       otlpEndpoint,
			nodeEnvServiceName:    deviceId, // temporary set the device id as well, so if opamp fails we can fallback to resolve k8s attributes in the collector
			nodeEnvNodeOptions:    nodeOptionsVal,
			nodeOdigosOpampServer: opampServerHost,
			nodeOdigosDeviceId:    deviceId,
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
