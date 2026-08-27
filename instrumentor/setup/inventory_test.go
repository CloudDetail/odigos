package setup

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestWorkloadInventoryReportsSupportedWorkloadsAndExcludesSystemNamespaces(t *testing.T) {
	t.Setenv("CURRENT_NS", "syncause")
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "orders"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "syncause"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "orders"}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "orders"}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "odiglet", Namespace: "syncause"}},
	).Build()
	manager := NewSetupManager(logr.Discard(), NewConfigStore(nil, true), client)
	inventory, err := manager.WorkloadInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Namespaces) != 1 || inventory.Namespaces[0] != "orders" {
		t.Fatalf("namespaces=%v", inventory.Namespaces)
	}
	if len(inventory.Workloads) != 2 || inventory.Workloads[0].Kind != "deployment" || inventory.Workloads[1].Kind != "statefulset" {
		t.Fatalf("workloads=%+v", inventory.Workloads)
	}
}
