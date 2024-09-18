package instrumentationdevice

import (
	"fmt"

	"github.com/odigos-io/odigos/common/consts"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const (
	OriginxInstrumentPatchAnnotations = "originx-instrument-patch"
)

type instrumentStatusChangePredicate struct {
	cfg *viper.Viper
	workloadEnvChangePredicate
}

// Update 对Update事件做初步过滤
// 忽略和注入完全无关的更新
func (w *instrumentStatusChangePredicate) Update(e event.UpdateEvent) bool {
	if w.cfg == nil || e.ObjectOld == nil || e.ObjectNew == nil {
		return w.workloadEnvChangePredicate.UpdateFunc(e)
	}

	// 检查是否发生了注入状态变更
	oldStatus := getInstrumentEnabledLabelFromObject(e.ObjectOld)
	newStatus := getInstrumentEnabledLabelFromObject(e.ObjectNew)

	expectedStatus := w.getExpectedInstrumentStatus(e.ObjectNew)

	if oldStatus != newStatus || newStatus != expectedStatus {
		return true
	}

	// 检查originx-instrument-patch是否就绪
	if !checkInstrumentPatchExists(e.ObjectNew) {
		return true
	}

	return w.workloadEnvChangePredicate.UpdateFunc(e)
}

func (w *instrumentStatusChangePredicate) getExpectedInstrumentStatus(obj client.Object) bool {
	if w.cfg.GetBool("force-instrument-all-namespace") {
		return true
	}

	workloadKey := fmt.Sprintf("workload.%s.%s", obj.GetNamespace(), getWorkloadKeyFromObject(obj))
	status := w.cfg.GetString(workloadKey)
	if status == "disabled" {
		return false
	}

	namespaceKey := fmt.Sprintf("namespace.%s", obj.GetNamespace())
	status = w.cfg.GetString(namespaceKey)
	if status == "enabled" || status == "enabledFuture" {
		return true
	}

	if w.cfg.GetBool("instrument-all-namespace") {
		return true
	}

	return false
}

func getInstrumentEnabledLabelFromObject(obj client.Object) bool {
	if labels := obj.GetLabels(); labels != nil {
		if labels[consts.OdigosInstrumentationLabel] == "enabled" {
			return true
		}
	}
	return false
}

func checkInstrumentPatchExists(obj client.Object) bool {
	if annos := obj.GetAnnotations(); annos != nil {
		if patch, find := annos[OriginxInstrumentPatchAnnotations]; find && len(patch) > 0 {
			return true
		}
	}
	return false
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
