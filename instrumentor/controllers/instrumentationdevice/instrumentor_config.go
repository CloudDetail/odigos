package instrumentationdevice

import (
	"os"
	"reflect"
	"strconv"

	"github.com/odigos-io/odigos/instrumentor/instrumentation"
)

func init() {
	CurrentInstrumentorConfig = SetCustomConfigByEnv(&CurrentInstrumentorConfig)
	instrumentation.OverwriteUserDefinedEnvs = CurrentInstrumentorConfig.ENABLE_OVERWRITE_USER_DEFINED_ENVS
}

var CurrentInstrumentorConfig InstrumentorConfig = InstrumentorConfig{
	ENABLE_CUSTOM_COLLECTOR:            true,
	ENABLE_OVERWRITE_USER_DEFINED_ENVS: false,
}

type InstrumentorConfig struct {
	ENABLE_CUSTOM_COLLECTOR            bool
	ENABLE_OVERWRITE_USER_DEFINED_ENVS bool
}

func SetCustomConfigByEnv(obj *InstrumentorConfig) InstrumentorConfig {
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
		case reflect.Bool:
			if value, find := os.LookupEnv(fieldName); find {
				opt, _ := strconv.ParseBool(value)
				field.SetBool(opt)
			}
		default:
			// do nothing
		}
	}
	return *obj
}
