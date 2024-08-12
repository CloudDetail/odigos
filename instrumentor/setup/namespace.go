package setup

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

/**
 *namespace:
 *  default: disabled
 *  originx: enabled
 *  kindling: enabledFuture
 *  - ...
 */

type NamespaceInstrumentRule struct {
}

func (r *NamespaceInstrumentRule) InstrumentAll(logger logr.Logger, c client.Client) ([]string, error) {
	// List all namespaces in the cluster using the client
	namespaceList := &corev1.NamespaceList{}
	err := c.List(context.Background(), namespaceList)
	if err != nil {
		return nil, err
	}

	var nsList []string

	// TODO deal with error
	for _, ns := range namespaceList.Items {
		if ns.Name == "kube-system" {
			// 永远不操作kube-system下面的资源
			continue
		}
		nsList = append(nsList, ns.Name)
		patch := getJsonMergePatchForInstrumentationLabel(true)
		logger.Info("instrument namespace", "name", ns.Name)
		if err := c.Patch(context.Background(), &ns, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return nil, err
		}
	}
	return nsList, nil
}

func (r *NamespaceInstrumentRule) InstrumentWithCfg(logger logr.Logger, c client.Client, cfg *viper.Viper, defaultEnable bool) ([]string, error) {
	nsCfg := cfg.GetStringMap("namespace")
	// List all namespaces in the cluster using the client
	namespaceList := &corev1.NamespaceList{}
	err := c.List(context.Background(), namespaceList)
	if err != nil {
		return nil, err
	}

	var nsList []string
	for _, ns := range namespaceList.Items {
		if ns.Name == "kube-system" {
			// 永远不操作kube-system下面的资源
			continue
		}
		logger.Info("check namespace", "name", ns.Name)
		nsList = append(nsList, ns.Name)
		op, find := nsCfg[ns.Name]
		isEnabled := checkIfEnabled(find, op, defaultEnable)
		if !isEnabled {
			value, find := ns.GetLabels()[consts.OdigosInstrumentationLabel]
			if !find || value == "disabled" {
				continue
			}
			logger.Info("uninstrument namespace", "name", ns.Name)
		} else {
			logger.Info("instrument namespace", "name", ns.Name)
		}
		patch := getJsonMergePatchForInstrumentationLabel(isEnabled)
		if err := c.Patch(context.Background(), &ns, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return nsList, err
		}
	}

	return nsList, nil
}

func checkIfEnabled(find bool, op any, def bool) bool {
	if !find {
		return def
	}
	operation, ok := op.(string)
	if !ok {
		return def
	}
	switch operation {
	case "enabledFuture":
		// 对Namespace来说,只有enabledFuture才设置instrument为true; 表示后续新增的工作负载全部注入
		return true
	case "disable":
		return false
	default:
		// 根据默认设置决定是否开启
		return def
	}
}
