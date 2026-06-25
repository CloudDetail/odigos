package instrumentationdevice

import (
	"testing"

	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildEnvNamePatchHintsForExistingEnvReplace(t *testing.T) {
	podSpec := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Env: []corev1.EnvVar{
						{Name: "OTEL_SERVICE_NAME", Value: "old"},
					},
				},
			},
		},
	}
	patches := []jsonpatch.Operation{
		{
			Operation: "replace",
			Path:      "/spec/containers/0/env/0/value",
			Value:     "new",
		},
		{
			Operation: "add",
			Path:      "/spec/containers/0/env/1",
			Value:     map[string]string{"name": "NEW", "value": "value"},
		},
	}

	hints := buildEnvNamePatchHints(podSpec, patches)
	if len(hints) != 1 {
		t.Fatalf("expected one hint for replace patch, got %d", len(hints))
	}
	hint := hints["/spec/containers/0/env/0/value"]
	if hint.ContainerName != "app" || hint.EnvName != "OTEL_SERVICE_NAME" || hint.Field != "value" {
		t.Fatalf("unexpected hint: %+v", hint)
	}
}
