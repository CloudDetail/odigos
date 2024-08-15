package instrumentationdevice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

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

		err = instrumentation.ApplyInstrumentationDevicesToPodTemplate(podSpec, runtimeDetails, odigosConfig.Spec.DefaultSDKs, deepCpObj)
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
			patchBytesBase64 := base64.StdEncoding.EncodeToString(patchBytes)
			if obj.GetAnnotations() == nil {
				obj.SetAnnotations(map[string]string{
					"originx-instrument-patch": patchBytesBase64,
				})
			} else {
				obj.GetAnnotations()["originx-instrument-patch"] = patchBytesBase64
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
		delete(annos, "originx-instrument-patch")
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
