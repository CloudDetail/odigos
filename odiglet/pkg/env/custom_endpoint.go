package env

import (
	"os"
	"reflect"
)

type CustomAgentConfig struct {
	OTEL_EXPORTER_OTLP_GRPC_ENDPOINT string
	OTEL_EXPORTER_OTLP_HTTP_ENDPOINT string
	OTEL_EXPORTER_OPAMP_ENDPOINT     string

	SW_AGENT_COLLECTOR_BACKEND_SERVICES string
	SW_LOGGING_OUTPUT                   string
	SW_LOGGING_DIR                      string
	SW_LOGGING_FILE_NAME                string
	SW_LOGGING_LEVEL                    string
	SW_LOGGING_MAX_HISTORY_FILES        string
	SW_LOGGING_MAX_FILE_SIZE            string
}

func DefaultCustomConfig() CustomAgentConfig {
	return CustomAgentConfig{
		SW_LOGGING_DIR: "/opt/skywalking/logs",
	}
}

func LoadCustomAgentConfig() CustomAgentConfig {
	cfg := DefaultCustomConfig()
	return SetCustomConfigByEnv(cfg)
}

func SetCustomConfigByEnv(obj CustomAgentConfig) CustomAgentConfig {
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
	return obj
}
