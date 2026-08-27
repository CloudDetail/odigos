package instrumentationdevice

import (
	"testing"

	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/instrumentor/setup"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreatePredicatePausesExplicitlyLabeledWorkloadWhileDaemonIsUnavailable(t *testing.T) {
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Namespace: "orders",
		Name:      "api",
		Labels:    map[string]string{consts.OdigosInstrumentationLabel: consts.InstrumentationEnabled},
	}}
	store := setup.NewConfigStore(viper.New(), true)
	if shouldInstrumentOnCreate(deployment, store) {
		t.Fatal("explicit workload label bypassed the daemon availability gate")
	}
	store.SetRemote(setup.NamespaceInjection{})
	if !shouldInstrumentOnCreate(deployment, store) {
		t.Fatal("explicit workload label was not accepted after daemon config recovery")
	}
}
