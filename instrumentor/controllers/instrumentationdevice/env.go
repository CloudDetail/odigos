package instrumentationdevice

import "os"

func init() {
	opt, ok := os.LookupEnv(ENABLE_CUSTOM_COLLECTOR_ENV)
	if !ok {
		return
	}

	if opt == "true" || opt == "1" || opt == "enabled" {
		ENABLE_CUSTOM_COLLECTOR = true
	}
}

const ENABLE_CUSTOM_COLLECTOR_ENV = "ENABLE_CUSTOM_COLLECTOR"

var ENABLE_CUSTOM_COLLECTOR = false
