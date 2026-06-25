package instrumentationdevice

import (
	"context"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type DeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("reconciling deployment for instrumentation", "namespace", req.Namespace, "name", req.Name)
	instrumentedAppName := workload.GetRuntimeObjectName(req.Name, "Deployment")
	err := reconcileSingleInstrumentedApplicationByName(ctx, r.Client, r.Scheme, instrumentedAppName, req.Namespace, req.Name, "Deployment")
	return ctrl.Result{}, err
}

type DaemonSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *DaemonSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("reconciling daemonset for instrumentation", "namespace", req.Namespace, "name", req.Name)
	instrumentedAppName := workload.GetRuntimeObjectName(req.Name, "DaemonSet")
	err := reconcileSingleInstrumentedApplicationByName(ctx, r.Client, r.Scheme, instrumentedAppName, req.Namespace, req.Name, "DaemonSet")
	return ctrl.Result{}, err
}

type StatefulSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *StatefulSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("reconciling statefulset for instrumentation", "namespace", req.Namespace, "name", req.Name)
	instrumentedAppName := workload.GetRuntimeObjectName(req.Name, "StatefulSet")
	err := reconcileSingleInstrumentedApplicationByName(ctx, r.Client, r.Scheme, instrumentedAppName, req.Namespace, req.Name, "StatefulSet")
	return ctrl.Result{}, err
}

func reconcileSingleInstrumentedApplicationByName(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, instrumentedAppName string, namespace string, workloadName string, workloadKind string) error {
	logger := log.FromContext(ctx)
	logger.Info("looking for exact instrumented application", "namespace", namespace, "instrumentedApplication", instrumentedAppName, "workloadName", workloadName, "workloadKind", workloadKind)
	var instrumentedApplication odigosv1.InstrumentedApplication
	err := k8sClient.Get(ctx, types.NamespacedName{Name: instrumentedAppName, Namespace: namespace}, &instrumentedApplication)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("exact instrumented application was not found, trying workload name regex rules", "namespace", namespace, "instrumentedApplication", instrumentedAppName, "workloadName", workloadName, "workloadKind", workloadKind)
			return reconcileInstrumentedApplicationByNameRegex(ctx, k8sClient, scheme, namespace, workloadName, workloadKind)
		}
		return err
	}
	logger.Info("exact instrumented application found, using existing runtime details", "namespace", namespace, "instrumentedApplication", instrumentedAppName, "workloadName", workloadName, "workloadKind", workloadKind, "runtimeDetails", len(instrumentedApplication.Spec.RuntimeDetails))
	return reconcileSingleInstrumentedApplication(ctx, k8sClient, &instrumentedApplication)
}
