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
			name:          "format applies regex to deployName",
			deployName:    "checkout-v2",
			containerName: "api",
			format:        "${deployName|^(?P<service>.*)-v[0-9]+$|$service}.${containerName}",
			want:          "checkout.api",
		},
		{
			name:          "format applies regex to containerName",
			deployName:    "checkout",
			containerName: "api-main",
			format:        "${deployName}.${containerName|^([^-]+).*$|$1}",
			want:          "checkout.api",
		},
		{
			name:          "invalid regex falls back to original variable value",
			deployName:    "checkout",
			containerName: "api",
			format:        "${deployName|[|ignored}.${containerName}",
			want:          "checkout.api",
		},
		{
			name:          "regex no match falls back to original variable value",
			deployName:    "checkout",
			containerName: "api",
			format:        "${deployName|^payment$|billing}.${containerName}",
			want:          "checkout.api",
		},
		{
			name:          "empty regex replacement falls back to original variable value",
			deployName:    "checkout",
			containerName: "api",
			format:        "${deployName|^checkout$|}.${containerName}",
			want:          "checkout.api",
		},
		{
			name:          "unknown variable falls back to default service name",
			deployName:    "checkout",
			containerName: "api",
			format:        "${service}.${containerName}",
			want:          "checkout-api",
		},
		{
			name:          "invalid regex expression falls back to default service name",
			deployName:    "checkout",
			containerName: "api",
			format:        "${deployName|^checkout$}.${containerName}",
			want:          "checkout-api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ServiceNameDefaultFormatEnv, tt.format)

			got := DefaultServiceName(tt.deployName, tt.containerName)
			assert.Equal(t, tt.want, got)
		})
	}
}
