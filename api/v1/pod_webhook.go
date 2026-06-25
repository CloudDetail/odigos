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
	"os"
	"strings"

	"github.com/odigos-io/odigos/common/consts"
	"gomodules.xyz/jsonpatch/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var podlog = logf.Log.WithName("patch-pod")

const InstrumentPatchUseIndexEnv = "ORIGINX_INSTRUMENT_PATCH_USE_INDEX"

type envNamePatchHint struct {
	ContainerName string `json:"containerName"`
	EnvName       string `json:"envName"`
	Field         string `json:"field"`
}

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
	podlog.Info("received mutating pod request", "namespace", req.Namespace, "name", req.Name, "containers", len(pod.Spec.Containers))

	// TODO 检查能否获取到可用的Odiglet示例; 如果Odiglet全部未就绪,则拒绝应用patch
	ownerReferences := pod.GetOwnerReferences()
	if len(ownerReferences) == 0 {
		podlog.Info("skip pod instrumentation because pod has no owner references", "namespace", req.Namespace, "name", req.Name)
		return admission.Allowed("No owner references")
	}

	ownerRef := ownerReferences[0]
	podlog.Info("resolved pod owner reference", "namespace", req.Namespace, "pod", req.Name, "ownerKind", ownerRef.Kind, "ownerName", ownerRef.Name)
	ownerName := ownerRef.Name
	ownerKind := ownerRef.Kind
	namespace := req.Namespace

	if ownerKind == "ReplicaSet" {
		// Or try to get ReplicaSet Owner
		// Since Other workload is not support yet, just inferred to be Deployment now
		idx := strings.LastIndex(ownerName, "-")
		if idx > 0 {
			ownerName = ownerName[:idx]
			ownerKind = "Deployment"
			podlog.Info("resolved replicaset owner to deployment", "namespace", namespace, "pod", req.Name, "deployment", ownerName)
		}
	}

	ownerObj, err := getOwnerObjFromKind(ownerKind)
	if err != nil {
		podlog.Info("owner references kind is not supported yet", "owner", ownerKind)
		return admission.Allowed("owner references kind is not supported yet: " + ownerKind)
	}

	err = a.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ownerName}, ownerObj)
	if err != nil {
		podlog.Info("cannot find owner", "namespace", namespace, "workloadKind", ownerKind, "workload", ownerName, "err", err)
		return admission.Allowed(fmt.Sprintf("can not find owner: %s/%s ", ownerKind, ownerName))
	}

	annotations, labels := getAnnotationsAndLabelsFromObj(ownerObj)
	if annotations == nil || labels == nil {
		podlog.Info("skip pod instrumentation because owner annotations or labels are empty", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "hasAnnotations", annotations != nil, "hasLabels", labels != nil)
		return admission.Allowed(fmt.Sprintf("no instrument annotations: %s/%s", ownerKind, ownerName))
	}

	// 检查工作负载上的patch
	patchB64, find := annotations[consts.InstrumentPatchAnnotation]
	if !find || len(patchB64) <= 0 {
		podlog.Info("skip pod instrumentation because owner has no instrument patch annotation", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "annotation", consts.InstrumentPatchAnnotation)
		return admission.Allowed(fmt.Sprintf("no instrument annotations: %s/%s", ownerKind, ownerName))
	}
	podlog.Info("found instrument patch annotation on owner", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "patchAnnotationBytesBase64", len(patchB64), "hasEnvNameHints", annotations[consts.InstrumentPatchEnvNamesAnnotation] != "")

	// 检查工作负载上的标签
	mark, find := labels[consts.OdigosInstrumentationLabel]
	if !find {
		// 再检查namespace上的标签
		namespaceObj := &corev1.Namespace{}
		err = a.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: namespace}, namespaceObj)
		if err != nil {
			podlog.Info("skip pod instrumentation because namespace object cannot be loaded", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "err", err)
			return admission.Allowed(fmt.Sprintf("can not find namespace: %s ", namespace))
		}

		mark, find := namespaceObj.GetAnnotations()[consts.OdigosInstrumentationLabel]
		if !find || mark != consts.InstrumentationEnabled {
			podlog.Info(fmt.Sprintf("instrument is not enabled for namespace: %s or workload: %s", namespace, ownerName))
			return admission.Allowed(fmt.Sprintf("instrument is not enabled for namespace: %s or workload: %s", namespace, ownerName))
		} else if mark == consts.InstrumentationDisabled {
			podlog.Info(fmt.Sprintf("instrument has been disabled for namespace: %s", namespace))
			return admission.Allowed(fmt.Sprintf("instrument has been disabled for namespace: %s", namespace))
		}
	} else if mark == consts.InstrumentationDisabled {
		podlog.Info("instrument has been disabled for workload", "workloadKind", ownerKind, "workload", ownerName)
		return admission.Allowed(fmt.Sprintf("instrument has been disabled for workload: %s, workload: %s", ownerKind, ownerName))
	}

	patchBytes, err := base64.StdEncoding.DecodeString(patchB64)
	if err != nil {
		msg := fmt.Sprintf("can not base64 decode originx-instrument-patch for %s/%s, err: %s", ownerKind, ownerName, err)
		podlog.Info(msg)
		return admission.Allowed(msg)
	}

	var patches []jsonpatch.Operation
	err = json.Unmarshal(patchBytes, &patches)
	if err != nil {
		msg := fmt.Sprintf("can not json unmarshal originx-instrument-patch for %s/%s, err: %s", ownerKind, ownerName, err)
		podlog.Info(msg)
		return admission.Allowed(msg)
	}
	podlog.Info("decoded instrument patch", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "patches", len(patches))

	useIndexPatch := useIndexBasedInstrumentPatch(annotations)
	if !useIndexPatch {
		hints := loadEnvNamePatchHints(annotations)
		podlog.Info("resolving env patch paths by env name", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "hints", len(hints), "patchesBeforeResolve", len(patches))
		patches = resolveEnvNamePatchPaths(pod, patches, hints)
		podlog.Info("resolved env patch paths by env name", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "patchesAfterResolve", len(patches))
	} else {
		podlog.Info("using index-based instrument patch paths", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName)
	}

	podlog.Info("instrument pod from odigos patch", "name", ownerName, "namespace", namespace, "pod", req.Name, "patches", len(patches), "indexBasedPatch", useIndexPatch)
	return admission.Patched("instrument patch", patches...)
}

func useIndexBasedInstrumentPatch(annotations map[string]string) bool {
	if isTruthy(os.Getenv(InstrumentPatchUseIndexEnv)) {
		return true
	}
	return isTruthy(annotations[consts.InstrumentPatchUseIndexAnnotation])
}

func isTruthy(value string) bool {
	switch strings.ToLower(value) {
	case "true", "1", "enabled", "yes":
		return true
	default:
		return false
	}
}

func loadEnvNamePatchHints(annotations map[string]string) map[string]envNamePatchHint {
	hintsB64 := annotations[consts.InstrumentPatchEnvNamesAnnotation]
	if hintsB64 == "" {
		return nil
	}
	hintsBytes, err := base64.StdEncoding.DecodeString(hintsB64)
	if err != nil {
		podlog.Info("can not base64 decode instrument patch env name hints", "err", err)
		return nil
	}
	var hints map[string]envNamePatchHint
	if err := json.Unmarshal(hintsBytes, &hints); err != nil {
		podlog.Info("can not json unmarshal instrument patch env name hints", "err", err)
		return nil
	}
	return hints
}

func resolveEnvNamePatchPaths(pod *corev1.Pod, patches []jsonpatch.Operation, hints map[string]envNamePatchHint) []jsonpatch.Operation {
	if len(hints) == 0 {
		return patches
	}

	resolved := make([]jsonpatch.Operation, 0, len(patches))
	for _, patch := range patches {
		hint, found := hints[patch.Path]
		if !found {
			resolved = append(resolved, patch)
			continue
		}

		path, ok := envNamePatchPathForPod(pod, hint)
		if !ok {
			podlog.Info("skip env patch because target env var was not found by name", "container", hint.ContainerName, "env", hint.EnvName, "originalPath", patch.Path)
			continue
		}
		podlog.Info("resolved env patch path by env name", "container", hint.ContainerName, "env", hint.EnvName, "originalPath", patch.Path, "resolvedPath", path)
		patch.Path = path
		resolved = append(resolved, patch)
	}
	return resolved
}

func envNamePatchPathForPod(pod *corev1.Pod, hint envNamePatchHint) (string, bool) {
	for containerIndex, container := range pod.Spec.Containers {
		if container.Name != hint.ContainerName {
			continue
		}
		for envIndex, envVar := range container.Env {
			if envVar.Name == hint.EnvName {
				return fmt.Sprintf("/spec/containers/%d/env/%d/%s", containerIndex, envIndex, hint.Field), true
			}
		}
		return "", false
	}
	return "", false
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
