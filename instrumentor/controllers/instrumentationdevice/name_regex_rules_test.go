package instrumentationdevice_test

import (
	"context"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/instrumentor/internal/testutil"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("workload name regex rules", func() {
	ctx := context.Background()
	var namespace *corev1.Namespace

	BeforeEach(func() {
		namespace = testutil.NewMockNamespace()
		Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
	})

	It("creates a concrete InstrumentedApplication for the latest matching workload from the newest historical version", func() {
		instrumentor := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "odigos-instrumentor",
				Namespace: consts.DefaultOdigosNamespace,
				Annotations: map[string]string{
					consts.WorkloadNameRegexRulesAnnotation: `[{"kinds":["Deployment"],"regex":"^(?P<base>.+)-[vV](?P<version>[0-9]+)$"}]`,
				},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "odigos-instrumentor"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "odigos-instrumentor"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "manager", Image: "test"}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, instrumentor)).Should(Succeed())

		v1 := testutil.SetOdigosInstrumentationEnabled(testutil.NewMockTestDeployment(namespace))
		v1.Name = "checkout-v1"
		Expect(k8sClient.Create(ctx, v1)).Should(Succeed())

		v1Ia := testutil.SetInstrumentedApplicationContainer(testutil.NewMockInstrumentedApplication(v1), nil, nil, common.PythonProgrammingLanguage)
		Expect(k8sClient.Create(ctx, v1Ia)).Should(Succeed())

		v2 := testutil.SetOdigosInstrumentationEnabled(testutil.NewMockTestDeployment(namespace))
		v2.Name = "checkout-v2"
		Expect(k8sClient.Create(ctx, v2)).Should(Succeed())

		v2IaName := workload.GetRuntimeObjectName(v2.Name, "Deployment")
		Eventually(func() bool {
			var v2Ia odigosv1.InstrumentedApplication
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: v2IaName}, &v2Ia)
			if err != nil {
				return false
			}
			return len(v2Ia.Spec.RuntimeDetails) == 1 &&
				v2Ia.Spec.RuntimeDetails[0].Language == common.PythonProgrammingLanguage
		}).Should(BeTrue())

		testutil.AssertInstrumentedApplicationRetained(ctx, k8sClient, v1Ia)
	})
})
