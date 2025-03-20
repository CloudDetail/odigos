package envOverwrite

import (
	"github.com/odigos-io/odigos/common"
)

// Since patches has been stored into annotations
// the environment variables in the Pod admission has user-defined part only.
// we will append the part defined by Odigos if needed
// return nil if no change needs
func PatchedEnvValueWithOdigosPart(envName string, envValue string, currentSdk common.OtelSdk) *string {
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

	mergedEnvValue := envValue + envMetadata.delim + desiredOdigosPart
	return &mergedEnvValue
}

// Compared to PatchedEnvValueWithOdigosPart, user-configured variables are no longer applied.
func PatchedEnvValueOnlyWithOdigosPart(envName string, currentSdk common.OtelSdk) *string {
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

	mergedEnvValue := desiredOdigosPart
	return &mergedEnvValue
}
