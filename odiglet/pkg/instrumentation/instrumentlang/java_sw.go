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
	swCollectorBackendServiceEnvVar = "SW_AGENT_COLLECTOR_BACKEND_SERVICES"
	swLOGGING_OUTPUT                = "SW_LOGGING_OUTPUT"
	swLOGGING_DIR                   = "SW_LOGGING_DIR"
	swLOGGING_FILE_NAME             = "SW_LOGGING_FILE_NAME"
	swLOGGING_MAX_FILE_SIZE         = "SW_LOGGING_MAX_FILE_SIZE"
	swLOGGING_MAX_HISTORY_FILES     = "SW_LOGGING_MAX_HISTORY_FILES"
	swMeterActiveEnvVar             = "SW_METER_ACTIVE"
)

func JavaInSkywalking(deviceId string, uniqueDestinationSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {
	javaOptsVal, _ := envOverwrite.ValToAppend(javaOptsEnvVar, common.SWSdkCommunity)
	javaToolOptionsVal, _ := envOverwrite.ValToAppend(javaToolOptionsEnvVar, common.SWSdkCommunity)

	var envs = map[string]string{
		javaToolOptionsEnvVar: javaToolOptionsVal,
		javaOptsEnvVar:        javaOptsVal,
	}

	if len(env.Current.APO_COLLECTOR_SKYWALKING_ENDPOINT) > 0 {
		envs[swCollectorBackendServiceEnvVar] = env.Current.APO_COLLECTOR_SKYWALKING_ENDPOINT
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

	if len(env.Current.SW_METER_ACTIVE) > 0 {
		envs[swMeterActiveEnvVar] = env.Current.SW_METER_ACTIVE
	}

	return &v1beta1.ContainerAllocateResponse{
		Envs: envs,
		Mounts: []*v1beta1.Mount{
			{
				ContainerPath: commonMountPath,
				HostPath:      commonMountPath,
				ReadOnly:      true,
			},
		},
	}
}
