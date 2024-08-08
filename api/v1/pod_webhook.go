/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gomodules.xyz/jsonpatch/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var podlog = logf.Log.WithName("patch-pod")

// +kubebuilder:webhook:path=/mutate-core-v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups=core,resources=pods,verbs=create,versions=v1,name=mpod.kb.io,admissionReviewVersions=v1
type PodInstrument struct {
	Client  client.Client
	Decoder admission.Decoder
}

func (a *PodInstrument) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}
	err := a.Decoder.Decode(req, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// TODO 检查能否获取到可用的Odiglet示例; 如果Odiglet全部未就绪,则拒绝应用patch
	ownerReferences := pod.GetOwnerReferences()
	if len(ownerReferences) == 0 {
		return admission.Allowed("No owner references")
	}

	ownerRef := ownerReferences[0]
	ownerName := ownerRef.Name
	ownerKind := ownerRef.Kind
	namespace := pod.Namespace

	if ownerKind == "ReplicaSet" {
		// Or try to get ReplicaSet Owner
		// Since Other workload is not support yet, just inferred to be Deployment now
		idx := strings.LastIndex(ownerName, "-")
		if idx > 0 {
			ownerName = ownerName[:idx]
			ownerKind = "Deployment"
		}
	}

	ownerObj, err := getOwnerObjFromKind(ownerKind)
	if err != nil {
		return admission.Allowed("owner references kind is not supported yet: " + ownerKind)
	}

	err = a.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ownerName}, ownerObj)
	if err != nil {
		return admission.Allowed(fmt.Sprintf("can not find owner: %s/%s ", ownerKind, ownerName))
	}

	annotations, labels := getAnnotationsAndLabelsFromObj(ownerObj)
	if annotations == nil || labels == nil {
		return admission.Allowed(fmt.Sprintf("no instrument annotations: %s/%s", ownerKind, ownerName))
	}

	value, find := labels["odigos-instrumentation"]
	if !find || value != "enabled" {
		return admission.Allowed(fmt.Sprintf("instrument has been disabled for application: %s/%s", ownerKind, ownerName))
	}

	patchB64, find := annotations["originx-instrument-patch"]
	if !find || len(patchB64) <= 0 {
		return admission.Allowed(fmt.Sprintf("no instrument annotations: %s/%s", ownerKind, ownerName))
	}

	patchBytes, err := base64.StdEncoding.DecodeString(patchB64)
	if err != nil {
		msg := fmt.Sprintf("can not base64 decode originx-instrument-patch for %s/%s, err: %s", ownerKind, ownerName, err)
		return admission.Allowed(msg)
	}

	var patches []jsonpatch.Operation
	err = json.Unmarshal(patchBytes, &patches)
	if err != nil {
		msg := fmt.Sprintf("can not json unmarshal originx-instrument-patch for %s/%s, err: %s", ownerKind, ownerName, err)
		return admission.Allowed(msg)
	}

	podlog.Info("instrument pod from odigos patch", "name", ownerName, "namespace", pod.Namespace)
	return admission.Patched("instrument patch", patches...)
}

func getOwnerObjFromKind(kind string) (client.Object, error) {
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

func getAnnotationsAndLabelsFromObj(obj client.Object) (map[string]string, map[string]string) {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		return o.GetAnnotations(), o.GetLabels()
	case *appsv1.StatefulSet:
		return o.GetAnnotations(), o.GetLabels()
	case *appsv1.DaemonSet:
		return o.GetAnnotations(), o.GetLabels()
	default:
		return nil, nil
	}
}
