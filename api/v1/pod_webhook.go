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
	"regexp"
	"strconv"
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

type workloadNameRegexRuleConfig struct {
	Kinds []string `json:"kinds,omitempty"`
	Regex string   `json:"regex"`
}

type podAdmissionNameRegexRule struct {
	kinds map[string]struct{}
	regex *regexp.Regexp
}

type podAdmissionNameMatch struct {
	base    string
	version int64
	obj     client.Object
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
	if annotations == nil {
		podlog.Info("owner annotations are empty, continuing with regex patch inheritance fallback", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName)
		annotations = map[string]string{}
	}
	if labels == nil {
		podlog.Info("owner labels are empty, continuing with namespace or inherited patch authorization", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName)
		labels = map[string]string{}
	}

	// 检查工作负载上的patch
	patchB64, find := annotations[consts.InstrumentPatchAnnotation]
	inheritedPatch := false
	if !find || len(patchB64) <= 0 {
		podlog.Info("owner has no instrument patch annotation, trying name regex patch inheritance", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "annotation", consts.InstrumentPatchAnnotation)
		inheritedAnnotations, inheritedFrom := a.inheritedPatchAnnotations(ctx, namespace, ownerKind, ownerName)
		if inheritedAnnotations == nil {
			podlog.Info("skip pod instrumentation because owner has no instrument patch annotation and no inherited patch was found", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "annotation", consts.InstrumentPatchAnnotation)
			return admission.Allowed(fmt.Sprintf("no instrument annotations: %s/%s", ownerKind, ownerName))
		}
		annotations = inheritedAnnotations
		patchB64 = annotations[consts.InstrumentPatchAnnotation]
		inheritedPatch = true
		podlog.Info("using inherited instrument patch for pod admission", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "inheritedFrom", inheritedFrom)
	}
	podlog.Info("found instrument patch annotation on owner", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName, "patchAnnotationBytesBase64", len(patchB64), "hasEnvNameHints", annotations[consts.InstrumentPatchEnvNamesAnnotation] != "")

	// 检查工作负载上的标签
	mark, find := labels[consts.OdigosInstrumentationLabel]
	if !find {
		if inheritedPatch {
			podlog.Info("allowing pod instrumentation without workload label because patch was inherited by regex", "namespace", namespace, "pod", req.Name, "workloadKind", ownerKind, "workload", ownerName)
		} else {
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

func (a *PodInstrument) inheritedPatchAnnotations(ctx context.Context, namespace string, kind string, name string) (map[string]string, string) {
	rules, err := loadPodAdmissionNameRegexRules()
	if err != nil {
		podlog.Info("failed to load workload name regex rules for pod admission patch inheritance", "namespace", namespace, "kind", kind, "name", name, "err", err)
		return nil, ""
	}
	if len(rules) == 0 {
		podlog.Info("no workload name regex rules configured for pod admission patch inheritance", "namespace", namespace, "kind", kind, "name", name, "env", consts.WorkloadNameRegexRulesEnv)
		return nil, ""
	}

	rule, currentMatch, matchedRules := selectPodAdmissionRegexRule(rules, name, kind)
	if rule == nil {
		podlog.Info("workload did not match name regex rules during pod admission", "namespace", namespace, "kind", kind, "name", name, "rules", len(rules))
		return nil, ""
	}
	if matchedRules > 1 {
		podlog.Info("workload matched multiple name regex rules during pod admission, using the last matching rule", "namespace", namespace, "kind", kind, "name", name, "matchedRules", matchedRules)
	}
	podlog.Info("workload matched name regex rule during pod admission", "namespace", namespace, "kind", kind, "name", name, "regex", rule.regex.String(), "base", currentMatch.base, "version", currentMatch.version)

	matches, err := a.listPodAdmissionRegexMatches(ctx, namespace, kind, rule, currentMatch.base)
	if err != nil {
		podlog.Info("failed to list workload name regex family during pod admission", "namespace", namespace, "kind", kind, "name", name, "base", currentMatch.base, "err", err)
		return nil, ""
	}

	var selected *podAdmissionNameMatch
	for i := range matches {
		if matches[i].obj.GetName() == name || matches[i].version >= currentMatch.version {
			continue
		}
		annotations := matches[i].obj.GetAnnotations()
		if annotations == nil || annotations[consts.InstrumentPatchAnnotation] == "" {
			continue
		}
		if selected == nil || matches[i].version > selected.version {
			selected = &matches[i]
		}
	}
	if selected == nil {
		podlog.Info("no historical workload with instrument patch found during pod admission", "namespace", namespace, "kind", kind, "name", name, "base", currentMatch.base, "familySize", len(matches))
		return nil, ""
	}

	podlog.Info("selected historical workload patch during pod admission", "namespace", namespace, "kind", kind, "name", name, "base", currentMatch.base, "sourceWorkload", selected.obj.GetName(), "sourceVersion", selected.version)
	return selected.obj.GetAnnotations(), selected.obj.GetName()
}

func loadPodAdmissionNameRegexRules() ([]podAdmissionNameRegexRule, error) {
	rawRules := strings.TrimSpace(os.Getenv(consts.WorkloadNameRegexRulesEnv))
	if rawRules == "" {
		return nil, nil
	}

	var configs []workloadNameRegexRuleConfig
	if err := json.Unmarshal([]byte(rawRules), &configs); err != nil {
		return nil, err
	}

	rules := make([]podAdmissionNameRegexRule, 0, len(configs))
	for _, cfg := range configs {
		compiled, err := regexp.Compile(cfg.Regex)
		if err != nil {
			return nil, err
		}
		if !hasRegexGroup(compiled, "base") || !hasRegexGroup(compiled, "version") {
			return nil, fmt.Errorf("workload name regex must include base and version named groups")
		}
		kinds := make(map[string]struct{}, len(cfg.Kinds))
		for _, kind := range cfg.Kinds {
			kinds[kind] = struct{}{}
		}
		rules = append(rules, podAdmissionNameRegexRule{
			kinds: kinds,
			regex: compiled,
		})
	}

	return rules, nil
}

func selectPodAdmissionRegexRule(rules []podAdmissionNameRegexRule, name string, kind string) (*podAdmissionNameRegexRule, *podAdmissionNameMatch, int) {
	var selectedRule *podAdmissionNameRegexRule
	var selectedMatch *podAdmissionNameMatch
	matches := 0
	for i := range rules {
		if !rules[i].matchesKind(kind) {
			continue
		}
		match, ok := rules[i].matchName(name)
		if !ok {
			continue
		}
		matches++
		selectedRule = &rules[i]
		selectedMatch = match
	}
	return selectedRule, selectedMatch, matches
}

func (r podAdmissionNameRegexRule) matchesKind(kind string) bool {
	if len(r.kinds) == 0 {
		return true
	}
	_, ok := r.kinds[kind]
	return ok
}

func (r podAdmissionNameRegexRule) matchName(name string) (*podAdmissionNameMatch, bool) {
	matches := r.regex.FindStringSubmatch(name)
	if matches == nil || matches[0] != name {
		return nil, false
	}
	groupNames := r.regex.SubexpNames()

	base := regexGroupValue(matches, groupNames, "base")
	versionValue := regexGroupValue(matches, groupNames, "version")
	if base == "" || versionValue == "" {
		return nil, false
	}
	version, err := strconv.ParseInt(versionValue, 10, 64)
	if err != nil {
		return nil, false
	}

	return &podAdmissionNameMatch{
		base:    base,
		version: version,
	}, true
}

func hasRegexGroup(regex *regexp.Regexp, name string) bool {
	for _, groupName := range regex.SubexpNames() {
		if groupName == name {
			return true
		}
	}
	return false
}

func regexGroupValue(matches []string, groupNames []string, name string) string {
	for i, groupName := range groupNames {
		if groupName == name && i < len(matches) {
			return matches[i]
		}
	}
	return ""
}

func (a *PodInstrument) listPodAdmissionRegexMatches(ctx context.Context, namespace string, kind string, rule *podAdmissionNameRegexRule, base string) ([]podAdmissionNameMatch, error) {
	objs, err := a.listWorkloadsByKind(ctx, namespace, kind)
	if err != nil {
		return nil, err
	}

	results := make([]podAdmissionNameMatch, 0)
	for _, obj := range objs {
		match, ok := rule.matchName(obj.GetName())
		if !ok || match.base != base {
			continue
		}
		match.obj = obj
		results = append(results, *match)
	}
	return results, nil
}

func (a *PodInstrument) listWorkloadsByKind(ctx context.Context, namespace string, kind string) ([]client.Object, error) {
	switch kind {
	case "Deployment":
		var list appsv1.DeploymentList
		if err := a.Client.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return nil, err
		}
		objs := make([]client.Object, 0, len(list.Items))
		for i := range list.Items {
			objs = append(objs, &list.Items[i])
		}
		return objs, nil
	case "StatefulSet":
		var list appsv1.StatefulSetList
		if err := a.Client.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return nil, err
		}
		objs := make([]client.Object, 0, len(list.Items))
		for i := range list.Items {
			objs = append(objs, &list.Items[i])
		}
		return objs, nil
	case "DaemonSet":
		var list appsv1.DaemonSetList
		if err := a.Client.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return nil, err
		}
		objs := make([]client.Object, 0, len(list.Items))
		for i := range list.Items {
			objs = append(objs, &list.Items[i])
		}
		return objs, nil
	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", kind)
	}
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
