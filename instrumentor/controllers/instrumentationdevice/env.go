package instrumentationdevice

import (
	"os"

	"github.com/odigos-io/odigos/instrumentor/instrumentation"
)

func init() {
	if opt, ok := os.LookupEnv(ENABLE_CUSTOM_COLLECTOR_ENV); ok {
		if opt == "true" || opt == "1" || opt == "enabled" {
			ENABLE_CUSTOM_COLLECTOR = true
		}
	}

	if opt, ok := os.LookupEnv(ENABLE_OVERWRITE_USER_DEFINED_ENVS_ENV); ok {
		if opt == "true" || opt == "1" || opt == "enabled" {
			ENABLE_OVERWRITE_USER_DEFINED_ENVS = true
			instrumentation.OverwriteUserDefinedEnvs = true
		}
	}
}

const ENABLE_CUSTOM_COLLECTOR_ENV = "ENABLE_CUSTOM_COLLECTOR"
const ENABLE_OVERWRITE_USER_DEFINED_ENVS_ENV = "ENABLE_OVERWRITE_USER_DEFINED_ENVS"

var ENABLE_CUSTOM_COLLECTOR = false
var ENABLE_OVERWRITE_USER_DEFINED_ENVS = false
