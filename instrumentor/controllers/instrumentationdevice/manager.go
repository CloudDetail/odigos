package instrumentationdevice

import (
	"fmt"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	v1 "github.com/odigos-io/odigos/api/v1"
	"github.com/odigos-io/odigos/instrumentor/setup"
	k8sutils "github.com/odigos-io/odigos/k8sutils/pkg/client"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type workloadNeedUpdateInstrument struct {
	cfg setup.ConfigReader
	workloadEnvChangePredicate
}

func shouldInstrumentOnCreate(obj client.Object, cfg setup.ConfigReader) bool {
	logger := ctrl.Log.WithName("instrumentationdevice-workload-filter")
	if cfg != nil && !cfg.InjectionAllowed() {
		logger.Info("workload create event ignored because namespace injection is paused", "namespace", obj.GetNamespace(), "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return false
	}
	if getInstrumentEnabledLabelFromObject(obj) {
		logger.Info("workload create event accepted because instrumentation label is enabled", "namespace", obj.GetNamespace(), "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return true
	}

	if cfg == nil {
		logger.Info("workload create event ignored because instrumentation label is not enabled and setup config is not available", "namespace", obj.GetNamespace(), "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return false
	}

	if cfg.GetBool("force-instrument-all-namespace") || cfg.GetBool("instrument-all-namespace") {
		logger.Info("workload create event accepted because namespace-wide instrumentation is enabled", "namespace", obj.GetNamespace(), "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return true
	}

	namespaceKey := fmt.Sprintf("namespace.%s", obj.GetNamespace())
	status := cfg.GetString(namespaceKey)
	accepted := status == "enabled" || status == "enabledFuture"
	logger.Info("workload create event evaluated namespace instrumentation config", "namespace", obj.GetNamespace(), "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind, "namespaceStatus", status, "accepted", accepted)
	return accepted
}

func (w *workloadNeedUpdateInstrument) Create(e event.CreateEvent) bool {
	return shouldInstrumentOnCreate(e.Object, w.cfg)
}

func (w *workloadNeedUpdateInstrument) Update(e event.UpdateEvent) bool {
	if w.cfg != nil && !w.cfg.InjectionAllowed() {
		return false
	}
	enabledOld := getInstrumentEnabledLabelFromObject(e.ObjectOld)
	enabledNew := getInstrumentEnabledLabelFromObject(e.ObjectNew)

	if !enabledOld && !enabledNew {
		// 不需要进行注入
		return false
	} else if enabledOld && !enabledNew {
		// 注入标签被移除,检查配置
		if w.cfg != nil {
			if w.cfg.GetBool("force-instrument-all-namespace") {
				return true
			}

			workloadKey := fmt.Sprintf("workload.%s.%s", e.ObjectOld.GetNamespace(), getWorkloadKeyFromObject(e.ObjectOld))
			status := w.cfg.GetString(workloadKey)
			if status == "enabled" {
				return true
			} else if status == "disabled" {
				return false
			}
			namespaceKey := fmt.Sprintf("namespace.%s", e.ObjectOld.GetNamespace())
			status = w.cfg.GetString(namespaceKey)
			if status == "enabled" || status == "enabledFuture" {
				return true
			} else if status == "disabled" {
				return false
			}

			if w.cfg.GetBool("instrument-all-namespace") {
				return true
			}
		}
		return false
	} else if !enabledOld && enabledNew {
		return true
	}

	return w.workloadEnvChangePredicate.Update(e)
}

type workloadEnvChangePredicate struct {
	predicate.Funcs
}

func (w workloadEnvChangePredicate) Create(e event.CreateEvent) bool {
	return shouldInstrumentOnCreate(e.Object, nil)
}

func (w workloadEnvChangePredicate) Update(e event.UpdateEvent) bool {

	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	oldPodSpec, err := getPodSpecFromObject(e.ObjectOld)
	if err != nil {
		return false
	}
	newPodSpec, err := getPodSpecFromObject(e.ObjectNew)
	if err != nil {
		return false
	}

	// only handle workloads if any env changed
	if len(oldPodSpec.Spec.Containers) != len(newPodSpec.Spec.Containers) {
		return true
	}
	for i := range oldPodSpec.Spec.Containers {
		if len(oldPodSpec.Spec.Containers[i].Env) != len(newPodSpec.Spec.Containers[i].Env) {
			return true
		}
		for j := range oldPodSpec.Spec.Containers[i].Env {
			prevEnv := &newPodSpec.Spec.Containers[i].Env[j]
			newEnv := &oldPodSpec.Spec.Containers[i].Env[j]
			if prevEnv.Name != newEnv.Name || prevEnv.Value != newEnv.Value {
				return true
			}
		}
	}

	return false
}

func (w workloadEnvChangePredicate) Delete(e event.DeleteEvent) bool {
	return false
}

func (w workloadEnvChangePredicate) Generic(e event.GenericEvent) bool {
	return false
}

func SetupWithManager(mgr ctrl.Manager, cfg setup.ConfigReader) error {
	// Create a new client with fallback to API server
	// We are doing this because client-go cache is not supporting dynamic cache rules
	// Sometimes we will need to get/list objects that are out of the cache (e.g. when namespace is labeled)
	clientWithFallback := k8sutils.NewKubernetesClientFromCacheWithAPIFallback(mgr.GetClient(), mgr.GetAPIReader())

	err := builder.
		ControllerManagedBy(mgr).
		For(&odigosv1.CollectorsGroup{}).
		Complete(&CollectorsGroupReconciler{
			Client: clientWithFallback,
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		For(&odigosv1.InstrumentedApplication{}).
		Complete(&InstrumentedApplicationReconciler{
			Client: clientWithFallback,
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		For(&odigosv1.OdigosConfiguration{}).
		Complete(&OdigosConfigReconciler{
			Client: clientWithFallback,
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		WithEventFilter(&workloadNeedUpdateInstrument{cfg: cfg}).
		Complete(&DeploymentReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		For(&appsv1.DaemonSet{}).
		WithEventFilter(&workloadNeedUpdateInstrument{cfg: cfg}).
		Complete(&DaemonSetReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}).
		WithEventFilter(&workloadNeedUpdateInstrument{cfg: cfg}).
		Complete(&StatefulSetReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	mgr.GetWebhookServer().Register("/mutate-core-v1-pod", &webhook.Admission{
		Handler: &v1.PodInstrument{
			Client:  clientWithFallback,
			Decoder: admission.NewDecoder(mgr.GetScheme()),
		},
	})

	return nil
}
