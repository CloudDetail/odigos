package setup

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// 读取启动配置,对现有的注入项进行设置
type SetupManager struct {
	Cfg    *viper.Viper
	client client.WithWatch
	logger logr.Logger

	namespaces NamespaceInstrumentRule
	workloads  WorkloadInstrumentRule

	instrumentAll      bool
	forceInstrumentAll bool

	mutex sync.Mutex
}

func NewSetupManager(logger logr.Logger, cfg *viper.Viper, client client.WithWatch) *SetupManager {
	setup := &SetupManager{
		Cfg:        cfg,
		client:     client,
		logger:     logger,
		namespaces: NamespaceInstrumentRule{},
		workloads:  WorkloadInstrumentRule{},
	}

	return setup
}

func (m *SetupManager) Start(ctx context.Context) error {
	m.UpdateAnnotationsByRule()
	m.logger.Info("setup manager sync config done")
	m.Cfg.WatchConfig()
	m.Cfg.OnConfigChange(func(e fsnotify.Event) {
		m.UpdateAnnotationsByRule()
		m.logger.Info("setup manager sync config done")
	})

	go m.WatchNamespace(ctx)
	return nil
}

func (m *SetupManager) WatchNamespace(ctx context.Context) {
	const (
		initialRetryInterval = 5 * time.Second
		maxRetryInterval     = 5 * time.Minute
	)

	retryInterval := initialRetryInterval
	for {
		select {
		case <-ctx.Done():
			m.logger.Info("stop watch namespace since stop signal")
			return
		default:
			if err := m.watchAndPatch(ctx); err != nil {
				m.logger.Error(err, "watch namespace is interrupted, wait for retry",
					"retryInterval(s)", int(retryInterval.Seconds()))
			} else {
				retryInterval = initialRetryInterval
				continue
			}

			select {
			case <-time.After(retryInterval):
				retryInterval = time.Duration(math.Min(float64(retryInterval)*2, float64(maxRetryInterval)))
			case <-ctx.Done():
				return
			}
		}
	}
}

func (m *SetupManager) watchAndPatch(ctx context.Context) error {
	watcher, err := m.client.Watch(
		ctx,
		&corev1.NamespaceList{},
		&client.ListOptions{},
	)
	if err != nil {
		return err
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			event, isOpen := <-watcher.ResultChan()
			if !isOpen {
				return errors.New("watch namespace is interrupted unexpected")
			}

			if event.Type != watch.Added {
				continue
			}

			nsObj, ok := event.Object.(*corev1.Namespace)
			if !ok {
				continue
			}
			if err := m.handleNamespaceCreation(ctx, nsObj); err != nil {
				m.logger.Error(err, "failed to handler namespace create", "namespace", nsObj.Name)
			}
		}
	}
}

func (m *SetupManager) handleNamespaceCreation(ctx context.Context, ns *corev1.Namespace) error {
	if ns.Name == "kube-system" || ns.Name == env.GetCurrentNamespace() {
		return nil
	}

	for _, nsName := range m.namespaces.checkedNamespace {
		if nsName == ns.Name {
			return nil
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.logger.Info("check namespace", "name", ns.Name)
	op, find := m.namespaces.nsCfg[ns.Name]
	isEnabled := checkIfEnabled(find, op, m.instrumentAll || m.forceInstrumentAll)
	skipNSPatch := false
	if !isEnabled {
		value, find := ns.GetLabels()[consts.OdigosInstrumentationLabel]
		if !find || value == "disabled" || value == "disable" {
			skipNSPatch = true
		} else {
			m.logger.Info("uninstrument namespace", "name", ns.Name)
		}
	} else {
		m.logger.Info("instrument namespace", "name", ns.Name)
	}

	if !skipNSPatch {
		patch := getJsonMergePatchForInstrumentationLabel(isEnabled)
		if err := m.client.Patch(context.Background(), ns, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return err
		}
	}

	m.namespaces.checkedNamespace = append(m.namespaces.checkedNamespace, ns.Name)
	err := m.workloads.InstrumentWithCfg(m.logger, m.client, ns.Name, m.Cfg, m.instrumentAll || m.forceInstrumentAll)
	if err != nil {
		m.logger.Error(err, "error instrument workload", "namespace", ns.Name)
	}
	return nil
}

func (m *SetupManager) UpdateAnnotationsByRule() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.instrumentAll = m.Cfg.GetBool("instrument-all-namespace")
	m.forceInstrumentAll = m.Cfg.GetBool("force-instrument-all-namespace")
	m.logger.Info(
		"instrument default cfg", "instrument-all-namespace", m.instrumentAll,
		"force-instrument-all-namespace", m.forceInstrumentAll,
	)
	// 强制注入所有的NS和workload
	if m.forceInstrumentAll {
		// 对所有可访问的NS(跳过kube-system)添加注入标记
		nsList, err := m.namespaces.InstrumentAll(m.logger, m.client)
		if err != nil {
			m.logger.Error(err, "instrument namespace failed")
		}
		// 同时向所有可访问的workload添加注入标记
		for _, namespace := range nsList {
			err = m.workloads.InstrumentAll(m.logger, m.client, namespace)
			if err != nil {
				m.logger.Error(err, "instrument workload failed")
			}
		}
		return
	}

	nsList, err := m.namespaces.InstrumentWithCfg(m.logger, m.client, m.Cfg, m.instrumentAll)
	if err != nil {
		m.logger.Error(err, "error instrument namespace")
	}

	for _, namespace := range nsList {
		err := m.workloads.InstrumentWithCfg(m.logger, m.client, namespace, m.Cfg, m.instrumentAll)
		if err != nil {
			m.logger.Error(err, "error instrument workload", "namespace", namespace)
		}
	}
}

func getJsonMergePatchForInstrumentationLabel(enabled bool) []byte {
	labelJsonMergePatchValue := "null"
	annotationsPatch := ""
	if enabled {
		labelJsonMergePatchValue = fmt.Sprintf("\"%s\"", consts.InstrumentationEnabled)
	} else {
		labelJsonMergePatchValue = fmt.Sprintf("\"%s\"", consts.InstrumentationDisabled)
		annotationsPatch = `,"annotations":{"originx-instrument-patch":null}`
	}

	jsonMergePatchContent := fmt.Sprintf(`{"metadata":{"labels":{"%s":%s}%s}}`, consts.OdigosInstrumentationLabel, labelJsonMergePatchValue, annotationsPatch)
	return []byte(jsonMergePatchContent)
}
