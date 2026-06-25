package instrumentationdevice

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const instrumentorDeploymentName = "odigos-instrumentor"

type workloadNameRegexRuleConfig struct {
	Kinds []string `json:"kinds,omitempty"`
	Regex string   `json:"regex"`
}

type workloadNameRegexRule struct {
	kinds map[string]struct{}
	regex *regexp.Regexp
}

type workloadNameMatch struct {
	base      string
	version   int64
	createdAt time.Time
	obj       client.Object
}

func reconcileInstrumentedApplicationByNameRegex(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, namespace string, workloadName string, workloadKind string) error {
	logger := log.FromContext(ctx)

	rules, err := loadWorkloadNameRegexRules(ctx, k8sClient)
	if err != nil {
		logger.Error(err, "error loading workload name regex rules")
		return nil
	}
	if len(rules) == 0 {
		logger.Info("no workload name regex rules configured", "namespace", namespace, "kind", workloadKind, "name", workloadName, "annotation", consts.WorkloadNameRegexRulesAnnotation)
		return nil
	}
	logger.Info("loaded workload name regex rules", "namespace", namespace, "kind", workloadKind, "name", workloadName, "rules", len(rules))

	rule, currentMatch, matchedRules := selectRegexRule(rules, workloadName, workloadKind)
	if rule == nil {
		logger.Info("workload did not match any name regex rule", "namespace", namespace, "kind", workloadKind, "name", workloadName, "rules", len(rules))
		return nil
	}
	logger.Info("workload matched name regex rule", "namespace", namespace, "kind", workloadKind, "name", workloadName, "regex", rule.regex.String(), "base", currentMatch.base, "version", currentMatch.version, "matchedRules", matchedRules)
	if matchedRules > 1 {
		logger.Info("workload matched multiple name regex rules, using the last matching rule", "namespace", namespace, "kind", workloadKind, "name", workloadName, "matchedRules", matchedRules)
	}

	matches, err := listWorkloadNameRegexMatches(ctx, k8sClient, namespace, workloadKind, rule, currentMatch.base)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		logger.Info("no workloads matched selected name regex family", "namespace", namespace, "kind", workloadKind, "name", workloadName, "regex", rule.regex.String(), "base", currentMatch.base)
		return nil
	}
	logger.Info("found workloads in name regex family", "namespace", namespace, "kind", workloadKind, "name", workloadName, "regex", rule.regex.String(), "base", currentMatch.base, "familySize", len(matches))

	latest := latestWorkloadMatch(matches)
	if latest == nil || latest.obj.GetName() != workloadName {
		latestName := ""
		latestVersion := int64(0)
		if latest != nil {
			latestName = latest.obj.GetName()
			latestVersion = latest.version
		}
		logger.Info("skip regex inheritance because workload is not the latest version in its name family", "namespace", namespace, "kind", workloadKind, "name", workloadName, "version", currentMatch.version, "latestName", latestName, "latestVersion", latestVersion)
		return nil
	}
	logger.Info("workload is latest version in name regex family, looking for historical instrumented application", "namespace", namespace, "kind", workloadKind, "name", workloadName, "version", currentMatch.version)

	source, err := latestHistoricalInstrumentedApplication(ctx, k8sClient, matches, workloadKind, workloadName)
	if err != nil {
		return err
	}
	if source == nil {
		logger.Info("no historical instrumented application found for regex inheritance", "namespace", namespace, "kind", workloadKind, "name", workloadName, "base", currentMatch.base)
		return nil
	}
	logger.Info("found historical instrumented application for regex inheritance", "namespace", namespace, "kind", workloadKind, "name", workloadName, "sourceInstrumentedApplication", source.Name, "runtimeDetails", len(source.Spec.RuntimeDetails))

	targetObj, err := getObjectFromKindString(workloadKind)
	if err != nil {
		return err
	}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: workloadName}, targetObj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	target := &odigosv1.InstrumentedApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workload.GetRuntimeObjectName(workloadName, workloadKind),
			Namespace: namespace,
		},
	}
	if err := controllerutil.SetControllerReference(targetObj, target, scheme); err != nil {
		return err
	}

	_, err = controllerutil.CreateOrPatch(ctx, k8sClient, target, func() error {
		target.Spec = *source.Spec.DeepCopy()
		return nil
	})
	if err != nil {
		return err
	}
	logger.Info("created or updated instrumented application from regex inheritance", "namespace", namespace, "kind", workloadKind, "name", workloadName, "targetInstrumentedApplication", target.Name, "sourceInstrumentedApplication", source.Name, "runtimeDetails", len(target.Spec.RuntimeDetails))

	return reconcileSingleInstrumentedApplication(ctx, k8sClient, target)
}

func loadWorkloadNameRegexRules(ctx context.Context, k8sClient client.Client) ([]workloadNameRegexRule, error) {
	var instrumentor appsv1.Deployment
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: env.GetCurrentNamespace(), Name: instrumentorDeploymentName}, &instrumentor); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	annotations := instrumentor.GetAnnotations()
	if annotations == nil {
		return nil, nil
	}
	rawRules := strings.TrimSpace(annotations[consts.WorkloadNameRegexRulesAnnotation])
	if rawRules == "" {
		return nil, nil
	}

	var configs []workloadNameRegexRuleConfig
	if err := json.Unmarshal([]byte(rawRules), &configs); err != nil {
		return nil, err
	}

	rules := make([]workloadNameRegexRule, 0, len(configs))
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
		rules = append(rules, workloadNameRegexRule{
			kinds: kinds,
			regex: compiled,
		})
	}

	return rules, nil
}

func selectRegexRule(rules []workloadNameRegexRule, name string, kind string) (*workloadNameRegexRule, *workloadNameMatch, int) {
	var selectedRule *workloadNameRegexRule
	var selectedMatch *workloadNameMatch
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

func (r workloadNameRegexRule) matchesKind(kind string) bool {
	if len(r.kinds) == 0 {
		return true
	}
	_, ok := r.kinds[kind]
	return ok
}

func (r workloadNameRegexRule) matchName(name string) (*workloadNameMatch, bool) {
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

	return &workloadNameMatch{
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

func listWorkloadNameRegexMatches(ctx context.Context, k8sClient client.Client, namespace string, kind string, rule *workloadNameRegexRule, base string) ([]workloadNameMatch, error) {
	objs, err := listWorkloadsByKind(ctx, k8sClient, namespace, kind)
	if err != nil {
		return nil, err
	}

	results := make([]workloadNameMatch, 0)
	for _, obj := range objs {
		match, ok := rule.matchName(obj.GetName())
		if !ok || match.base != base {
			continue
		}
		match.obj = obj
		match.createdAt = obj.GetCreationTimestamp().Time
		results = append(results, *match)
	}
	return results, nil
}

func listWorkloadsByKind(ctx context.Context, k8sClient client.Client, namespace string, kind string) ([]client.Object, error) {
	switch kind {
	case "Deployment":
		var list appsv1.DeploymentList
		if err := k8sClient.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return nil, err
		}
		objs := make([]client.Object, 0, len(list.Items))
		for i := range list.Items {
			objs = append(objs, &list.Items[i])
		}
		return objs, nil
	case "StatefulSet":
		var list appsv1.StatefulSetList
		if err := k8sClient.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return nil, err
		}
		objs := make([]client.Object, 0, len(list.Items))
		for i := range list.Items {
			objs = append(objs, &list.Items[i])
		}
		return objs, nil
	case "DaemonSet":
		var list appsv1.DaemonSetList
		if err := k8sClient.List(ctx, &list, client.InNamespace(namespace)); err != nil {
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

func latestWorkloadMatch(matches []workloadNameMatch) *workloadNameMatch {
	if len(matches) == 0 {
		return nil
	}
	latest := &matches[0]
	for i := 1; i < len(matches); i++ {
		if isMatchNewer(&matches[i], latest) {
			latest = &matches[i]
		}
	}
	return latest
}

func isMatchNewer(candidate *workloadNameMatch, current *workloadNameMatch) bool {
	if candidate.version != current.version {
		return candidate.version > current.version
	}
	if !candidate.createdAt.Equal(current.createdAt) {
		return candidate.createdAt.After(current.createdAt)
	}
	return candidate.obj.GetName() > current.obj.GetName()
}

func latestHistoricalInstrumentedApplication(ctx context.Context, k8sClient client.Client, matches []workloadNameMatch, kind string, currentWorkloadName string) (*odigosv1.InstrumentedApplication, error) {
	var selectedMatch *workloadNameMatch
	var selectedIa *odigosv1.InstrumentedApplication

	for i := range matches {
		if matches[i].obj.GetName() == currentWorkloadName {
			continue
		}
		iaName := workload.GetRuntimeObjectName(matches[i].obj.GetName(), kind)
		var ia odigosv1.InstrumentedApplication
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: matches[i].obj.GetNamespace(), Name: iaName}, &ia)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if selectedMatch == nil || isMatchNewer(&matches[i], selectedMatch) {
			selectedMatch = &matches[i]
			selectedIa = ia.DeepCopy()
		}
	}

	return selectedIa, nil
}
