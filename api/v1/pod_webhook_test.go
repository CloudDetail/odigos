package v1

import (
	"testing"

	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
)

func TestResolveEnvNamePatchPathsUsesCurrentEnvIndex(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Env: []corev1.EnvVar{
						{Name: "FIRST", Value: "first"},
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
	}
	hints := map[string]envNamePatchHint{
		"/spec/containers/0/env/0/value": {
			ContainerName: "app",
			EnvName:       "OTEL_SERVICE_NAME",
			Field:         "value",
		},
	}

	resolved := resolveEnvNamePatchPaths(pod, patches, hints)
	if len(resolved) != 1 {
		t.Fatalf("expected one resolved patch, got %d", len(resolved))
	}
	if resolved[0].Path != "/spec/containers/0/env/1/value" {
		t.Fatalf("expected patch path to use current env index, got %s", resolved[0].Path)
	}
}

func TestResolveEnvNamePatchPathsSkipsMissingEnv(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
			},
		},
	}
	patches := []jsonpatch.Operation{
		{
			Operation: "replace",
			Path:      "/spec/containers/0/env/0/value",
			Value:     "new",
		},
	}
	hints := map[string]envNamePatchHint{
		"/spec/containers/0/env/0/value": {
			ContainerName: "app",
			EnvName:       "OTEL_SERVICE_NAME",
			Field:         "value",
		},
	}

	resolved := resolveEnvNamePatchPaths(pod, patches, hints)
	if len(resolved) != 0 {
		t.Fatalf("expected missing env patch to be skipped, got %d patches", len(resolved))
	}
}
