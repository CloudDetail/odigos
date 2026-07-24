package instrumentlang

import (
	"fmt"
	"runtime"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/odiglet/pkg/env"
	"github.com/odigos-io/odigos/odiglet/pkg/instrumentation/consts"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	enableProfilingEnvVar = "CORECLR_ENABLE_PROFILING"
	profilerEndVar        = "CORECLR_PROFILER"
	profilerId            = "{918728DD-259F-4A6A-AC2B-B85E1B658318}"
	profilerPathEnv       = "CORECLR_PROFILER_PATH"
	profilerPathAMD64     = "/var/odigos/dotnet/linux-x64/OpenTelemetry.AutoInstrumentation.Native.so"
	profilerPathARM64     = "/var/odigos/dotnet/linux-arm64/OpenTelemetry.AutoInstrumentation.Native.so"
	serviceNameEnv        = "OTEL_SERVICE_NAME"
	collectorUrlEnv       = "OTEL_EXPORTER_OTLP_ENDPOINT"
	tracerHomeEnv         = "OTEL_DOTNET_AUTO_HOME"
	exportTypeEnv         = "OTEL_TRACES_EXPORTER"
	exportProtocolEnv     = "OTEL_EXPORTER_OTLP_PROTOCOL"
	tracerHome            = "/var/odigos/dotnet"
	resourceAttrEnv       = "OTEL_RESOURCE_ATTRIBUTES"
	startupHookEnv        = "DOTNET_STARTUP_HOOKS"
	startupHook           = "/var/odigos/dotnet/net/OpenTelemetry.AutoInstrumentation.StartupHook.dll"
	additonalDepsEnv      = "DOTNET_ADDITIONAL_DEPS"
	additonalDeps         = "/var/odigos/dotnet/AdditionalDeps"
	sharedStoreEnv        = "DOTNET_SHARED_STORE"
	sharedStore           = "/var/odigos/dotnet/store"
)

func dotNetProfilerPath() string {
	if runtime.GOARCH == "arm64" {
		return profilerPathARM64
	}
	return profilerPathAMD64
}

func DotNet(deviceId string, uniqueDestinationSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {
	collectorUrlValue := fmt.Sprintf("http://%s:%d", env.Current.NodeIP, consts.OTLPHttpPort)
	if len(env.Current.APO_COLLECTOR_HTTP_ENDPOINT) > 0 {
		collectorUrlValue = env.Current.APO_COLLECTOR_HTTP_ENDPOINT
	}

	return &v1beta1.ContainerAllocateResponse{
		Envs: map[string]string{
			enableProfilingEnvVar: "1",
			profilerEndVar:        profilerId,
			profilerPathEnv:       dotNetProfilerPath(),
			tracerHomeEnv:         tracerHome,
			collectorUrlEnv:       collectorUrlValue,
			serviceNameEnv:        deviceId,
			exportTypeEnv:         "otlp",
			exportProtocolEnv:     "http/protobuf",
			resourceAttrEnv:       "odigos.device=dotnet",
			startupHookEnv:        startupHook,
			additonalDepsEnv:      additonalDeps,
			sharedStoreEnv:        sharedStore,
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
