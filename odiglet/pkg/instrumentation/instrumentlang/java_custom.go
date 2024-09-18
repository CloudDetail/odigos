package instrumentlang

import (
	"fmt"
	"os"
	"strings"

	"github.com/odigos-io/odigos/common"
	"gopkg.in/ini.v1"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func JavaInCustomAgent(deviceId string, uniqueDestinationSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {
	envs, err := customEnvs("/var/odigos/custom/libapoinstrument.conf")
	if err != nil {
		envs = make(map[string]string)
	}
	return &v1beta1.ContainerAllocateResponse{
		Envs: envs,
		Mounts: []*v1beta1.Mount{
			{
				ContainerPath: "/etc/apo/instrumentations/",
				HostPath:      "/var/odigos/",
				ReadOnly:      true,
			},
		},
	}
}

func customEnvs(path string) (map[string]string, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, err
	}

	defaultEnv := GetDefaultInternalValue()

	var envs = make(map[string]string)
	section, err := cfg.GetSection("java")
	if err != nil {
		return nil, err
	}
	for _, key := range section.Keys() {
		rawValue := strings.TrimSpace(key.Value())
		if strings.HasPrefix(rawValue, "{{") && strings.HasSuffix(rawValue, "}}") {
			val := strings.TrimPrefix(rawValue, "{{")
			val = strings.TrimSuffix(val, "}}")

			envVal, find := os.LookupEnv(strings.TrimSpace(val))
			if find {
				envs[key.Name()] = envVal
			} else if v, find := defaultEnv[strings.TrimSpace(val)]; find {
				envs[key.Name()] = v
			} else {
				envs[key.Name()] = key.Value()
			}
		} else {
			envs[key.Name()] = key.Value()
		}
	}

	return envs, nil
}

func GetDefaultInternalValue() map[string]string {
	val, find := os.LookupEnv("NODE_IP")
	if !find {
		val = "localhost"
	}

	return map[string]string{
		"APO_COLLECTOR_GRPC_ENDPOINT":       fmt.Sprintf("http://%s:%d", val, 4317),
		"APO_COLLECTOR_HTTP_ENDPOINT":       fmt.Sprintf("http://%s:%d", val, 4318),
		"APO_COLLECTOR_SKYWALKING_ENDPOINT": fmt.Sprintf("%s:%d", val, 11800),
	}
}
