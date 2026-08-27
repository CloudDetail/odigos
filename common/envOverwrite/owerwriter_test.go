package envOverwrite

import (
	"testing"

	"github.com/odigos-io/odigos/common"
	"github.com/stretchr/testify/assert"
)

func TestGetPatchedEnvValue(t *testing.T) {
	nodeOptionsNativeCommunity, _ := ValToAppend("NODE_OPTIONS", common.OtelSdkNativeCommunity)
	nodeOptionsEbpfEnterprise, _ := ValToAppend("NODE_OPTIONS", common.OtelSdkEbpfEnterprise)
	userVal := "--max-old-space-size=4096"

	// test different cases
	tests := []struct {
		name                 string
		envName              string
		observedValue        string
		sdk                  common.OtelSdk
		patchedValueExpected string
	}{
		{
			name:                 "un-relevant env var",
			envName:              "PATH",
			observedValue:        "/usr/local/bin:/usr/bin:/bin",
			sdk:                  common.OtelSdkNativeCommunity,
			patchedValueExpected: "",
		},
		{
			name:                 "only user value",
			envName:              "NODE_OPTIONS",
			observedValue:        userVal,
			sdk:                  common.OtelSdkNativeCommunity,
			patchedValueExpected: userVal + " " + nodeOptionsNativeCommunity,
		},
		{
			name:                 "only odigos value",
			envName:              "NODE_OPTIONS",
			observedValue:        nodeOptionsNativeCommunity,
			sdk:                  common.OtelSdkNativeCommunity,
			patchedValueExpected: "",
		},
		{
			name:                 "user value with odigos value matching SDKs",
			envName:              "NODE_OPTIONS",
			observedValue:        userVal + " " + nodeOptionsNativeCommunity,
			sdk:                  common.OtelSdkNativeCommunity,
			patchedValueExpected: userVal + " " + nodeOptionsNativeCommunity,
		},
		{
			name:                 "user value with odigos value with different SDKs",
			envName:              "NODE_OPTIONS",
			observedValue:        userVal + " " + nodeOptionsNativeCommunity,
			sdk:                  common.OtelSdkEbpfEnterprise,
			patchedValueExpected: userVal + " " + nodeOptionsEbpfEnterprise,
		},
		{
			// No user values are observed, hence there is not need to patch
			// even if the observed value is different from the SDK value
			name:                 "observed odigos value different from SDK",
			envName:              "NODE_OPTIONS",
			observedValue:        nodeOptionsNativeCommunity,
			sdk:                  common.OtelSdkEbpfEnterprise,
			patchedValueExpected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patchedValue := GetPatchedEnvValue(tt.envName, tt.observedValue, tt.sdk)
			if patchedValue == nil {
				assert.Equal(t, tt.patchedValueExpected, "", "mismatch in GetPatchedEnvValue: %s", tt.name)
			} else {
				assert.Equal(t, tt.patchedValueExpected, *patchedValue, "mismatch in GetPatchedEnvValue: %s", tt.name)
			}
		})
	}

}

func TestDefaultServiceName(t *testing.T) {
	tests := []struct {
		name          string
		deployName    string
		containerName string
		format        string
		envs          map[string]string
		want          string
	}{
		{
			name:          "default multi container",
			deployName:    "checkout",
			containerName: "api",
			want:          "checkout-api",
		},
		{
			name:          "default single container",
			deployName:    "checkout",
			containerName: "checkout",
			want:          "checkout",
		},
		{
			name:          "format with predefined variables",
			deployName:    "checkout",
			containerName: "api",
			format:        "${deployName}.${containerName}",
			want:          "checkout.api",
		},
		{
			name:          "format uses derived variable from workloadName",
			deployName:    "checkout-v2",
			containerName: "api",
			format:        "${app}.${containerName}",
			envs: map[string]string{
				"ODIGOS_SERVICE_NAME_VAR_APP_SOURCE":      "workloadName",
				"ODIGOS_SERVICE_NAME_VAR_APP_REGEX":       "^(?P<service>.*)-v[0-9]+$",
				"ODIGOS_SERVICE_NAME_VAR_APP_REPLACEMENT": "$service",
			},
			want: "checkout.api",
		},
		{
			name:          "format uses derived variable from containerName",
			deployName:    "checkout",
			containerName: "api-main",
			format:        "${deployName}.${role}",
			envs: map[string]string{
				"ODIGOS_SERVICE_NAME_VAR_ROLE_SOURCE":      "containerName",
				"ODIGOS_SERVICE_NAME_VAR_ROLE_REGEX":       "^([^-]+).*$",
				"ODIGOS_SERVICE_NAME_VAR_ROLE_REPLACEMENT": "$1",
			},
			want: "checkout.api",
		},
		{
			name:          "derived variable supports snake case env names",
			deployName:    "checkout",
			containerName: "api-main",
			format:        "${appRole}",
			envs: map[string]string{
				"ODIGOS_SERVICE_NAME_VAR_APP_ROLE_SOURCE":      "containerName",
				"ODIGOS_SERVICE_NAME_VAR_APP_ROLE_REGEX":       "^([^-]+).*$",
				"ODIGOS_SERVICE_NAME_VAR_APP_ROLE_REPLACEMENT": "$1",
			},
			want: "api",
		},
		{
			name:          "derived variable supports workflowName source alias",
			deployName:    "checkout-v2",
			containerName: "api",
			format:        "${app}.${containerName}",
			envs: map[string]string{
				"ODIGOS_SERVICE_NAME_VAR_APP_SOURCE":      "workflowName",
				"ODIGOS_SERVICE_NAME_VAR_APP_REGEX":       "^(.*)-v[0-9]+$",
				"ODIGOS_SERVICE_NAME_VAR_APP_REPLACEMENT": "$1",
			},
			want: "checkout.api",
		},
		{
			name:          "invalid regex falls back to source value",
			deployName:    "checkout",
			containerName: "api",
			format:        "${app}.${containerName}",
			envs: map[string]string{
				"ODIGOS_SERVICE_NAME_VAR_APP_SOURCE":      "workloadName",
				"ODIGOS_SERVICE_NAME_VAR_APP_REGEX":       "[",
				"ODIGOS_SERVICE_NAME_VAR_APP_REPLACEMENT": "ignored",
			},
			want: "checkout.api",
		},
		{
			name:          "regex no match falls back to source value",
			deployName:    "checkout",
			containerName: "api",
			format:        "${app}.${containerName}",
			envs: map[string]string{
				"ODIGOS_SERVICE_NAME_VAR_APP_SOURCE":      "workloadName",
				"ODIGOS_SERVICE_NAME_VAR_APP_REGEX":       "^payment$",
				"ODIGOS_SERVICE_NAME_VAR_APP_REPLACEMENT": "billing",
			},
			want: "checkout.api",
		},
		{
			name:          "empty regex replacement falls back to source value",
			deployName:    "checkout",
			containerName: "api",
			format:        "${app}.${containerName}",
			envs: map[string]string{
				"ODIGOS_SERVICE_NAME_VAR_APP_SOURCE":      "workloadName",
				"ODIGOS_SERVICE_NAME_VAR_APP_REGEX":       "^checkout$",
				"ODIGOS_SERVICE_NAME_VAR_APP_REPLACEMENT": "",
			},
			want: "checkout.api",
		},
		{
			name:          "unknown variable falls back to default service name",
			deployName:    "checkout",
			containerName: "api",
			format:        "${service}.${containerName}",
			want:          "checkout-api",
		},
		{
			name:          "missing derived variable source falls back to default service name",
			deployName:    "checkout",
			containerName: "api",
			format:        "${app}.${containerName}",
			envs: map[string]string{
				"ODIGOS_SERVICE_NAME_VAR_APP_REGEX":       "^checkout$",
				"ODIGOS_SERVICE_NAME_VAR_APP_REPLACEMENT": "billing",
			},
			want: "checkout-api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ServiceNameDefaultFormatEnv, tt.format)
			for key, value := range tt.envs {
				t.Setenv(key, value)
			}

			got := DefaultServiceName(tt.deployName, tt.containerName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRemoteServiceNamePolicyKeepsInstrumentorEnvironmentNames(t *testing.T) {
	t.Setenv(ServiceNameEnvNamesEnv, "LOCAL_SERVICE")
	t.Setenv(ServiceNameDefaultFormatEnv, "local-${containerName}")
	SetServiceNamePolicy(&ServiceNamePolicy{
		DefaultFormat: "${app}.${containerName}",
		Variables: []ServiceNameVariable{{
			Name: "app", Source: "workloadName", Regex: "^checkout$", Replacement: "billing",
		}},
	})
	t.Cleanup(func() { SetServiceNamePolicy(nil) })

	envNames, ok := ServiceNameEnv(common.OtelSdkNativeCommunity)
	assert.True(t, ok)
	assert.Contains(t, envNames, "OTEL_SERVICE_NAME")
	assert.Contains(t, envNames, "LOCAL_SERVICE")
	assert.Equal(t, "billing.api", DefaultServiceName("checkout", "api"))
}

func TestRegexServiceNameMappingHasHighestPriority(t *testing.T) {
	SetServiceNamePolicy(&ServiceNamePolicy{
		DefaultFormat: "formatted-${workloadName}",
		Mappings: []ServiceNameMapping{{
			NamespacePattern: "^prod$", WorkloadPattern: "^checkout-.*$", ServiceName: "checkout",
		}},
	})
	t.Cleanup(func() { SetServiceNamePolicy(nil) })
	assert.Equal(t, "checkout", ServiceName("prod", "checkout-api", "api"))
	assert.Equal(t, "formatted-checkout-api", ServiceName("staging", "checkout-api", "api"))
}
