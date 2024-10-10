package env

import (
	"os"
	"reflect"
)

type CustomAgentConfig struct {
	APO_COLLECTOR_GRPC_ENDPOINT  string
	APO_COLLECTOR_HTTP_ENDPOINT  string
	OTEL_EXPORTER_OPAMP_ENDPOINT string

	OTEL_TRACES_EXPORTER  string
	OTEL_METRICS_EXPORTER string
	OTEL_LOGS_EXPORTER    string

	APO_COLLECTOR_SKYWALKING_ENDPOINT string
	SW_LOGGING_OUTPUT                 string
	SW_LOGGING_DIR                    string
	SW_LOGGING_FILE_NAME              string
	SW_LOGGING_LEVEL                  string
	SW_LOGGING_MAX_HISTORY_FILES      string
	SW_LOGGING_MAX_FILE_SIZE          string
	SW_METER_ACTIVE                   string
}

func DefaultCustomConfig() CustomAgentConfig {
	return CustomAgentConfig{
		SW_LOGGING_DIR: "/opt/skywalking/logs",
	}
}

func LoadCustomAgentConfig() CustomAgentConfig {
	cfg := DefaultCustomConfig()
	return SetCustomConfigByEnv(&cfg)
}

func SetCustomConfigByEnv(obj *CustomAgentConfig) CustomAgentConfig {
	val := reflect.ValueOf(obj).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := field.Type()
		fieldName := typ.Field(i).Name

		switch fieldType.Kind() {
		case reflect.String:
			if value, find := os.LookupEnv(fieldName); find {
				field.SetString(value)
			}
		default:
			// do nothing
		}
	}
	return *obj
}
