package setup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

/**
 *workload:
 *  namespace1:
 *    deployment/workload1: enabled
 *    ...
 *  namespace2:
 *    deployment/workload2: disabled
 *    statefulset/workload1: true
 *    ...
 *  namespace3:
 *    daemonset/workload3: true
 *    ...
 */

type WorkloadInstrumentRule struct {
}

func (r *WorkloadInstrumentRule) InstrumentAll(logger logr.Logger, c client.Client, namespace string) error {
	statefulset := &appsv1.StatefulSetList{}
	if err := c.List(context.Background(), statefulset, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, statefulset := range statefulset.Items {
		patch := getJsonMergePatchForInstrumentationLabel(true)
		logger.Info("instrument statefulset", "namespace", statefulset.Namespace, "name", statefulset.Name)
		if err := c.Patch(context.Background(), &statefulset, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return err
		}
	}

	deployments := &appsv1.DeploymentList{}
	if err := c.List(context.Background(), deployments, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, deployment := range deployments.Items {
		patch := getJsonMergePatchForInstrumentationLabel(true)
		logger.Info("instrument deployment", "namespace", deployment.Namespace, "name", deployment.Name)
		if err := c.Patch(context.Background(), &deployment, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return err
		}
	}

	daemonsets := &appsv1.DaemonSetList{}
	if err := c.List(context.Background(), daemonsets, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, daemonset := range daemonsets.Items {
		patch := getJsonMergePatchForInstrumentationLabel(true)
		logger.Info("instrument daemonset", "namespace", daemonset.Namespace, "name", daemonset.Name)
		if err := c.Patch(context.Background(), &daemonset, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return err
		}
	}

	return nil
}

func (r *WorkloadInstrumentRule) InstrumentWithCfg(logger logr.Logger, c client.Client, namespace string, cfg *viper.Viper, defaultEnable bool) error {
	workloadCfg := cfg.GetStringMap("workload")
	var namespacedCfg map[string]any
	if cfg, find := workloadCfg[namespace]; find {
		namespacedCfg = cfg.(map[string]any)
	} else {
		namespacedCfg = make(map[string]any)
	}

	if defaultEnable {
		// 如果设置了全局Enabled,检查当前namespace是否设置了disabled
		nsCfg := cfg.GetStringMap("namespace")
		if operation, find := nsCfg[namespace]; find {
			if operation.(string) == "disabled" {
				defaultEnable = false
			}
		}
	} else {
		// 如果未设置全局Enabled,检查当前Namespace的配置
		nsCfg := cfg.GetStringMap("namespace")
		if operation, find := nsCfg[namespace]; find {
			if operation.(string) == "enabled" || operation.(string) == "enabledFuture" {
				defaultEnable = true
			}
		}
	}

	statefulset := &appsv1.StatefulSetList{}
	if err := c.List(context.Background(), statefulset, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, statefulset := range statefulset.Items {
		op, find := namespacedCfg[getWorkloadKeyFromObject(&statefulset)]
		isEnabled := checkIsWorkloadEnabled(find, op, defaultEnable)
		if !isEnabled {
			value, find := statefulset.GetLabels()[consts.OdigosInstrumentationLabel]
			if find && value == "disabled" {
				continue
			} else if !find && !defaultEnable {
				continue
			}

			logger.Info("uninstrument statefulset", "namespace", statefulset.Namespace, "name", statefulset.Name)
		} else {
			logger.Info("instrument statefulset", "namespace", statefulset.Namespace, "name", statefulset.Name)
		}
		patch := getJsonMergePatchForInstrumentationLabel(isEnabled)
		if err := c.Patch(context.Background(), &statefulset, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return err
		}
	}

	deployments := &appsv1.DeploymentList{}
	if err := c.List(context.Background(), deployments, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, deployment := range deployments.Items {
		op, find := namespacedCfg[getWorkloadKeyFromObject(&deployment)]
		if find {
			logger.Info("find deployment config", "namespace", deployment.Namespace, "name", deployment.Name)
		} else {
			logger.Info("not find deployment config", "workloadId", getWorkloadKeyFromObject(&deployment))
		}
		isEnabled := checkIsWorkloadEnabled(find, op, defaultEnable)
		if !isEnabled {
			value, find := deployment.GetLabels()[consts.OdigosInstrumentationLabel]
			if !find || value == "disabled" {
				continue
			}
			logger.Info("uninstrument deployment", "namespace", deployment.Namespace, "name", deployment.Name)
		} else {
			logger.Info("instrument deployment", "namespace", deployment.Namespace, "name", deployment.Name)
		}
		patch := getJsonMergePatchForInstrumentationLabel(isEnabled)

		logger.Info("patch deployment", "namespace", deployment.Namespace, "name", deployment.Name, "patch", string(patch))
		if err := c.Patch(context.Background(), &deployment, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return err
		}
	}

	daemonsets := &appsv1.DaemonSetList{}
	if err := c.List(context.Background(), daemonsets, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, daemonset := range daemonsets.Items {
		op, find := namespacedCfg[getWorkloadKeyFromObject(&daemonset)]
		isEnabled := checkIsWorkloadEnabled(find, op, defaultEnable)
		if !isEnabled {
			value, find := daemonset.GetLabels()[consts.OdigosInstrumentationLabel]
			if !find || value == "disabled" {
				continue
			}
			logger.Info("uninstrument daemonset", "namespace", daemonset.Namespace, "name", daemonset.Name)
		} else {
			logger.Info("instrument daemonset", "namespace", daemonset.Namespace, "name", daemonset.Name)
		}
		patch := getJsonMergePatchForInstrumentationLabel(isEnabled)
		if err := c.Patch(context.Background(), &daemonset, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return err
		}
	}

	return nil
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

func checkIsWorkloadEnabled(find bool, op any, def bool) bool {
	if !find {
		return def
	}
	operation, ok := op.(string)
	if !ok {
		return def
	}
	switch operation {
	case "disabled":
		return false
	case "enabled":
		return true
	default:
		return def
	}
}
