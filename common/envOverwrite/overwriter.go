package envOverwrite

import (
	"os"
	"regexp"
	"strings"

	"github.com/odigos-io/odigos/common"
)

const ServiceNameEnvNamesEnv = "ODIGOS_SERVICE_NAME_ENV_NAMES"
const ServiceNameDefaultFormatEnv = "ODIGOS_SERVICE_NAME_DEFAULT_FORMAT"
const serviceNameVarPrefix = "ODIGOS_SERVICE_NAME_VAR_"
const serviceNameVarSourceSuffix = "_SOURCE"
const serviceNameVarRegexSuffix = "_REGEX"
const serviceNameVarReplacementSuffix = "_REPLACEMENT"

type envValues struct {
	delim  string
	values map[common.OtelSdk]string
}

// EnvValuesMap is a map of environment variables odigos uses for various languages and goals.
// The key is the environment variable name and the value is the value to be set or appended
// to the environment variable. We need to make sure that in case any of these environment
// variables is already set, we append the value to it instead of overwriting it.
//
// Note: The values here needs to be in sync with the paths used in the odigos images.
// If the paths are changed in the odigos images, the values here should be updated accordingly.
var EnvValuesMap = map[string]envValues{
	"NODE_OPTIONS": {
		delim: " ",
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "--require /var/odigos/nodejs/autoinstrumentation.js",
			common.OtelSdkEbpfEnterprise:  "--require /var/odigos/nodejs-ebpf/autoinstrumentation.js",
		},
	},
	"PYTHONPATH": {
		delim: ":",
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "/var/odigos/python:/var/odigos/python/opentelemetry/instrumentation/auto_instrumentation",
			common.OtelSdkEbpfEnterprise:  "/var/odigos/python-ebpf:/var/odigos/python/opentelemetry/instrumentation/auto_instrumentation:/var/odigos/python",
		},
	},
	"JAVA_OPTS": {
		delim: " ",
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "-javaagent:/var/odigos/java/javaagent.jar",
			common.OtelSdkEbpfEnterprise:  "-javaagent:/var/odigos/java-ebpf/dtrace-injector.jar",
			common.OtelSdkNativeEnterprise: "-javaagent:/var/odigos/java-ext-ebpf/javaagent.jar " +
				"-Dotel.javaagent.extensions=/var/odigos/java-ext-ebpf/otel_agent_extension.jar",

			common.SWSdkCommunity: "-javaagent:/var/odigos/skywalking/java/skywalking-agent.jar",
		},
	},
	"JAVA_TOOL_OPTIONS": {
		delim: " ",
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "-javaagent:/var/odigos/java/javaagent.jar",
			common.OtelSdkEbpfEnterprise:  "-javaagent:/var/odigos/java-ebpf/dtrace-injector.jar",
			common.OtelSdkNativeEnterprise: "-javaagent:/var/odigos/java-ext-ebpf/javaagent.jar " +
				"-Dotel.javaagent.extensions=/var/odigos/java-ext-ebpf/otel_agent_extension.jar",
			common.SWSdkCommunity: "-javaagent:/var/odigos/skywalking/java/skywalking-agent.jar",
		},
	},
}

func GetPatchedEnvValue(envName string, observedValue string, currentSdk common.OtelSdk) *string {
	envMetadata, ok := EnvValuesMap[envName]
	if !ok {
		// Odigos does not manipulate this environment variable, so ignore it
		return nil
	}

	desiredOdigosPart, ok := envMetadata.values[currentSdk]
	if !ok {
		// No specific overwrite is required for this SDK
		return nil
	}

	// scenario 1: no user defined values and no odigos value
	// happens: might be the case right after the source is instrumented, and before the instrumentation is applied.
	// action: there are no user defined values, so no need to make any changes.
	if observedValue == "" {
		return nil
	}

	// scenario 2: no user defined values, only odigos value
	// happens: when the user did not set any value to this env (either via manifest or dockerfile)
	// action: we don't need to overwrite the value, just let odigos handle it
	for _, sdkEnvValue := range envMetadata.values {
		if sdkEnvValue == observedValue {
			return nil
		}
	}

	// Scenario 3: both odigos and user defined values are present
	// happens: when the user set some values to this env (either via manifest or dockerfile) and odigos instrumentation is applied.
	// action: we want to keep the user defined values and upsert the odigos value.
	for _, sdkEnvValue := range envMetadata.values {
		if strings.Contains(observedValue, sdkEnvValue) {
			if sdkEnvValue == desiredOdigosPart {
				// shortcut, the value is already patched
				// both the odigos part equals to the new value, and the user part we want to keep
				return &observedValue
			} else {
				// The environment variable is patched by some other odigos sdk.
				// replace just the odigos part with the new desired value.
				// this can happen when moving between SDKs.
				patchedEvnValue := strings.ReplaceAll(observedValue, sdkEnvValue, desiredOdigosPart)
				return &patchedEvnValue
			}
		}
	}

	// Scenario 4: only user defined values are present
	// happens: when the user set some values to this env (either via manifest or dockerfile) and odigos instrumentation not yet applied.
	// action: we want to keep the user defined values and append the odigos value.
	mergedEnvValue := observedValue + envMetadata.delim + desiredOdigosPart
	return &mergedEnvValue
}

func ValToAppend(envName string, sdk common.OtelSdk) (string, bool) {
	env, exists := EnvValuesMap[envName]
	if !exists {
		return "", false
	}

	valToAppend, ok := env.values[sdk]
	if !ok {
		return "", false
	}

	return valToAppend, true
}

func ServiceNameEnv(sdk common.OtelSdk) ([]string, bool) {
	var envNames []string
	switch sdk.SdkType {
	case common.NativeOtelSdkType, common.EbpfOtelSdkType:
		envNames = []string{"OTEL_SERVICE_NAME", "EDAS_AHAS_APPNAME"}
	case common.SWSdkType:
		envNames = []string{"SW_AGENT_NAME", "EDAS_AHAS_APPNAME"}
	case common.CustomSdkType:
		// 不知道会用哪种SDK,姑且全部添加已知的ServiceName
		envNames = []string{"OTEL_SERVICE_NAME", "SW_AGENT_NAME", "EDAS_AHAS_APPNAME"}
	}

	envNames = appendServiceNameEnvNamesFromEnv(envNames)
	return envNames, len(envNames) > 0
}

func appendServiceNameEnvNamesFromEnv(envNames []string) []string {
	customEnvNames, ok := os.LookupEnv(ServiceNameEnvNamesEnv)
	if !ok {
		return envNames
	}

	existing := make(map[string]struct{}, len(envNames))
	for _, envName := range envNames {
		existing[envName] = struct{}{}
	}

	for _, envName := range strings.Split(customEnvNames, ",") {
		envName = strings.TrimSpace(envName)
		if envName == "" {
			continue
		}
		if _, found := existing[envName]; found {
			continue
		}
		envNames = append(envNames, envName)
		existing[envName] = struct{}{}
	}

	return envNames
}

func DefaultServiceName(deployName string, containerName string) string {
	defaultName := defaultServiceName(deployName, containerName)
	format, formatFound := os.LookupEnv(ServiceNameDefaultFormatEnv)
	if !formatFound || format == "" {
		return defaultName
	}

	variables := map[string]string{
		"workloadName":  deployName,
		"workflowName":  deployName,
		"deployName":    deployName,
		"containerName": containerName,
	}
	addServiceNameDerivedVariables(variables)

	rendered, ok := renderServiceNameFormat(format, variables)
	if !ok || rendered == "" {
		return defaultName
	}

	return rendered
}

func defaultServiceName(deployName string, containerName string) string {
	if deployName != containerName {
		return deployName + "-" + containerName
	}
	return containerName
}

func renderServiceNameFormat(format string, variables map[string]string) (string, bool) {
	ok := true
	rendered := os.Expand(format, func(expression string) string {
		value, valueOk := renderServiceNameVariable(expression, variables)
		if !valueOk {
			ok = false
		}
		return value
	})
	return rendered, ok
}

func renderServiceNameVariable(expression string, variables map[string]string) (string, bool) {
	value, ok := variables[expression]
	return value, ok
}

type serviceNameDerivedVariable struct {
	source      string
	regex       string
	replacement string
}

func addServiceNameDerivedVariables(variables map[string]string) {
	derivedVariables := serviceNameDerivedVariablesFromEnv()
	for variableName, variableConfig := range derivedVariables {
		sourceValue, ok := serviceNameVariableSourceValue(variableConfig.source, variables)
		if !ok {
			continue
		}

		variables[variableName] = renderServiceNameDerivedVariable(sourceValue, variableConfig)
	}
}

func serviceNameDerivedVariablesFromEnv() map[string]serviceNameDerivedVariable {
	derivedVariables := make(map[string]serviceNameDerivedVariable)
	for _, envEntry := range os.Environ() {
		key, value, found := strings.Cut(envEntry, "=")
		if !found || !strings.HasPrefix(key, serviceNameVarPrefix) {
			continue
		}

		variableName, field, ok := serviceNameDerivedVariableEnvField(key)
		if !ok {
			continue
		}

		variableConfig := derivedVariables[variableName]
		switch field {
		case "source":
			variableConfig.source = value
		case "regex":
			variableConfig.regex = value
		case "replacement":
			variableConfig.replacement = value
		}
		derivedVariables[variableName] = variableConfig
	}
	return derivedVariables
}

func serviceNameDerivedVariableEnvField(envName string) (string, string, bool) {
	envName = strings.TrimPrefix(envName, serviceNameVarPrefix)

	switch {
	case strings.HasSuffix(envName, serviceNameVarSourceSuffix):
		return serviceNameVariableName(strings.TrimSuffix(envName, serviceNameVarSourceSuffix)), "source", true
	case strings.HasSuffix(envName, serviceNameVarRegexSuffix):
		return serviceNameVariableName(strings.TrimSuffix(envName, serviceNameVarRegexSuffix)), "regex", true
	case strings.HasSuffix(envName, serviceNameVarReplacementSuffix):
		return serviceNameVariableName(strings.TrimSuffix(envName, serviceNameVarReplacementSuffix)), "replacement", true
	default:
		return "", "", false
	}
}

func serviceNameVariableName(envVariableName string) string {
	parts := strings.Split(strings.ToLower(envVariableName), "_")
	if len(parts) == 0 {
		return ""
	}

	variableName := parts[0]
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		variableName += strings.ToUpper(part[:1]) + part[1:]
	}
	return variableName
}

func serviceNameVariableSourceValue(source string, variables map[string]string) (string, bool) {
	switch source {
	case "workloadName", "workflowName", "deployName", "containerName":
		value, ok := variables[source]
		return value, ok
	default:
		return "", false
	}
}

func renderServiceNameDerivedVariable(sourceValue string, variableConfig serviceNameDerivedVariable) string {
	if variableConfig.regex == "" || variableConfig.replacement == "" {
		return sourceValue
	}

	regex, err := regexp.Compile(variableConfig.regex)
	if err != nil {
		return sourceValue
	}

	if !regex.MatchString(sourceValue) {
		return sourceValue
	}

	rendered := regex.ReplaceAllString(sourceValue, variableConfig.replacement)
	if rendered == "" {
		return sourceValue
	}

	return rendered
}
