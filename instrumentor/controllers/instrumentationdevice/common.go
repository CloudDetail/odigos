package instrumentationdevice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/instrumentor/instrumentation"
	"github.com/odigos-io/odigos/k8sutils/pkg/conditions"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	"gomodules.xyz/jsonpatch/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type UnInstrumentReason string

const (
	UnInstrumentReasonDataCollectionNotReady UnInstrumentReason = "DataCollection not ready"
	UnInstrumentReasonNoRuntimeDetails       UnInstrumentReason = "No runtime details"
	UnInstrumentReasonRemoveAll              UnInstrumentReason = "Remove all"
)

const (
	appliedInstrumentationDeviceType = "AppliedInstrumentationDevice"
)

func clearInstrumentationEbpf(obj client.Object) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return
	}

	delete(annotations, consts.EbpfInstrumentationAnnotation)
}

func isDataCollectionReady(ctx context.Context, c client.Client) bool {
	logger := log.FromContext(ctx)
	var collectorGroups odigosv1.CollectorsGroupList
	err := c.List(ctx, &collectorGroups, client.InNamespace(env.GetCurrentNamespace()))
	if err != nil {
		logger.Error(err, "error getting collectors groups, skipping instrumentation")
		return false
	}

	for _, cg := range collectorGroups.Items {
		// up until v1.0.31, the collectors group role names were "GATEWAY" and "DATA_COLLECTION".
		// in v1.0.32, the role names were changed to "CLUSTER_GATEWAY" and "NODE_COLLECTOR",
		// due to adding the Processor CRD which uses these role names.
		// the new names are more descriptive and are preparations for future roles.
		// the check for "DATA_COLLECTION" is a temporary support for users that upgrade from <=v1.0.31 to >=v1.0.32.
		// once we drop support for <=v1.0.31, we can remove this comparison.
		if (cg.Spec.Role == odigosv1.CollectorsGroupRoleNodeCollector || cg.Spec.Role == "DATA_COLLECTION") && cg.Status.Ready {
			return true
		}
	}

	return false
}

func instrument(logger logr.Logger, ctx context.Context, kubeClient client.Client, runtimeDetails *odigosv1.InstrumentedApplication) error {
	obj, err := getTargetObject(ctx, kubeClient, runtimeDetails)
	if err != nil {
		return err
	}

	var odigosConfig odigosv1.OdigosConfiguration
	err = kubeClient.Get(ctx, client.ObjectKey{Namespace: env.GetCurrentNamespace(), Name: consts.OdigosConfigurationName}, &odigosConfig)
	if err != nil {
		return err
	}

	result, err := controllerutil.CreateOrPatch(ctx, kubeClient, obj, func() error {
		deepCpObj := obj.DeepCopyObject().(client.Object)

		podSpec, err := getPodSpecFromObject(deepCpObj)
		if err != nil {
			return err
		}

		err = instrumentation.ApplyInstrumentationDevicesToPodTemplate(logger, podSpec, runtimeDetails, odigosConfig.Spec.DefaultSDKs, deepCpObj)
		if err != nil {
			return err
		}

		rawPodSpec, err := getPodSpecFromObject(obj)
		if err != nil {
			return err
		}
		rawMarshaledPodSpec, err := json.Marshal(rawPodSpec)
		if err != nil {
			return err
		}
		marshaledPodSpec, err := json.Marshal(podSpec)
		if err != nil {
			return err
		}
		patches, err := jsonpatch.CreatePatch(rawMarshaledPodSpec, marshaledPodSpec)
		if err != nil {
			return err
		}
		patchBytes, err := json.Marshal(patches)
		if err != nil {
			return err
		}
		if len(patchBytes) > 0 {
			patchEnvNameHints := buildEnvNamePatchHints(rawPodSpec, patches)
			patchBytesBase64 := base64.StdEncoding.EncodeToString(patchBytes)
			patchEnvNameHintsBytes, err := json.Marshal(patchEnvNameHints)
			if err != nil {
				return err
			}
			patchEnvNameHintsBase64 := base64.StdEncoding.EncodeToString(patchEnvNameHintsBytes)
			logger.Info("generated instrument patch for workload", "namespace", obj.GetNamespace(), "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind, "patches", len(patches), "envNameHints", len(patchEnvNameHints))
			if obj.GetAnnotations() == nil {
				obj.SetAnnotations(map[string]string{
					consts.InstrumentPatchAnnotation:         patchBytesBase64,
					consts.InstrumentPatchEnvNamesAnnotation: patchEnvNameHintsBase64,
				})
			} else {
				obj.GetAnnotations()[consts.InstrumentPatchAnnotation] = patchBytesBase64
				obj.GetAnnotations()[consts.InstrumentPatchEnvNamesAnnotation] = patchEnvNameHintsBase64
			}

			if obj.GetLabels() == nil {
				obj.SetLabels(map[string]string{
					consts.OdigosInstrumentationLabel: consts.InstrumentationEnabled,
				})
			} else {
				obj.GetLabels()[consts.OdigosInstrumentationLabel] = consts.InstrumentationEnabled
			}
		}

		return nil
	})

	if err != nil {
		conditions.UpdateStatusConditions(ctx, kubeClient, runtimeDetails, &runtimeDetails.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, "ErrApplyInstrumentationDevice", err.Error())
		return err
	}
	conditions.UpdateStatusConditions(ctx, kubeClient, runtimeDetails, &runtimeDetails.Status.Conditions, metav1.ConditionTrue, appliedInstrumentationDeviceType, string(result), "Successfully applied instrumentation device to pod template")

	if result != controllerutil.OperationResultNone {
		logger.V(0).Info("instrumented application", "name", obj.GetName(), "namespace", obj.GetNamespace())
	}

	return nil
}

func uninstrument(logger logr.Logger, ctx context.Context, kubeClient client.Client, namespace string, name string, kind string, reason UnInstrumentReason) error {
	obj, err := getObjectFromKindString(kind)
	if err != nil {
		logger.Error(err, "error getting object from kind string")
		return err
	}

	err = kubeClient.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		logger.Error(err, "error getting object")
		return err
	}

	result, err := controllerutil.CreateOrPatch(ctx, kubeClient, obj, func() error {
		annos := obj.GetAnnotations()
		delete(annos, consts.InstrumentPatchAnnotation)
		delete(annos, consts.InstrumentPatchEnvNamesAnnotation)
		return nil
	})

	if err != nil {
		return err
	}

	if result != controllerutil.OperationResultNone {
		logger.V(0).Info("uninstrumented application", "name", obj.GetName(), "namespace", obj.GetNamespace(), "reason", reason)
	}

	return nil
}

type EnvNamePatchHint struct {
	ContainerName string `json:"containerName"`
	EnvName       string `json:"envName"`
	Field         string `json:"field"`
}

func buildEnvNamePatchHints(rawPodSpec *corev1.PodTemplateSpec, patches []jsonpatch.Operation) map[string]EnvNamePatchHint {
	hints := make(map[string]EnvNamePatchHint)
	for _, patch := range patches {
		if patch.Operation != "replace" {
			continue
		}
		containerIndex, envIndex, envField, ok := parseEnvFieldPatchPath(patch.Path)
		if !ok {
			continue
		}
		if containerIndex < 0 || containerIndex >= len(rawPodSpec.Spec.Containers) {
			continue
		}
		container := rawPodSpec.Spec.Containers[containerIndex]
		if envIndex < 0 || envIndex >= len(container.Env) {
			continue
		}
		hints[patch.Path] = EnvNamePatchHint{
			ContainerName: container.Name,
			EnvName:       container.Env[envIndex].Name,
			Field:         envField,
		}
	}
	return hints
}

func parseEnvFieldPatchPath(path string) (containerIndex int, envIndex int, envField string, ok bool) {
	parts := strings.Split(path, "/")
	if len(parts) != 7 ||
		parts[0] != "" ||
		parts[1] != "spec" ||
		parts[2] != "containers" ||
		parts[4] != "env" ||
		parts[6] == "" {
		return 0, 0, "", false
	}
	containerIndex, err := strconv.Atoi(parts[3])
	if err != nil {
		return 0, 0, "", false
	}
	envIndex, err = strconv.Atoi(parts[5])
	if err != nil {
		return 0, 0, "", false
	}
	envField = parts[6]
	return containerIndex, envIndex, envField, true
}

func getTargetObject(ctx context.Context, kubeClient client.Client, runtimeDetails *odigosv1.InstrumentedApplication) (client.Object, error) {
	name, kind, err := workload.GetWorkloadInfoRuntimeName(runtimeDetails.Name)
	if err != nil {
		return nil, err
	}

	obj, err := getObjectFromKindString(kind)
	if err != nil {
		return nil, err
	}

	err = kubeClient.Get(ctx, client.ObjectKey{
		Namespace: runtimeDetails.Namespace,
		Name:      name,
	}, obj)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func getPodSpecFromObject(obj client.Object) (*corev1.PodTemplateSpec, error) {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		return &o.Spec.Template, nil
	case *appsv1.StatefulSet:
		return &o.Spec.Template, nil
	case *appsv1.DaemonSet:
		return &o.Spec.Template, nil
	default:
		return nil, errors.New("unknown kind")
	}
}

func getInstrumentEnabledLabelFromObject(obj client.Object) bool {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		if labels := o.GetLabels(); labels != nil {
			if labels[consts.OdigosInstrumentationLabel] == "enabled" {
				return true
			}
		}
		return false
	case *appsv1.StatefulSet:
		if labels := o.GetLabels(); labels != nil {
			if labels[consts.OdigosInstrumentationLabel] == "enabled" {
				return true
			}
		}
		return false
	case *appsv1.DaemonSet:
		if labels := o.GetLabels(); labels != nil {
			if labels[consts.OdigosInstrumentationLabel] == "enabled" {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func getObjectFromKindString(kind string) (client.Object, error) {
	switch kind {
	case "Deployment":
		return &appsv1.Deployment{}, nil
	case "StatefulSet":
		return &appsv1.StatefulSet{}, nil
	case "DaemonSet":
		return &appsv1.DaemonSet{}, nil
	default:
		return nil, errors.New("unknown kind")
	}
}

func getWorkloadKeyFromObject(obj client.Object) string {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		return fmt.Sprintf("deployment/%s", o.Name)
	case *appsv1.StatefulSet:
		return fmt.Sprintf("statefulset/%s", o.Name)
	case *appsv1.DaemonSet:
		return fmt.Sprintf("daemonset/%s", o.Name)
	default:
		return ""
	}
}
