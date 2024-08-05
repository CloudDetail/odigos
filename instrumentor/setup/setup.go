package setup

import (
	"context"
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// 读取启动配置,对现有的注入项进行设置
type SetupManager struct {
	cfg    *viper.Viper
	client client.Client
	logger logr.Logger

	namespaces NamespaceInstrumentRule
	workloads  WorkloadInstrumentRule
}

func NewSetupManager(logger logr.Logger, cfg *viper.Viper, client client.Client) *SetupManager {
	setup := &SetupManager{
		cfg:        cfg,
		client:     client,
		logger:     logger,
		namespaces: NamespaceInstrumentRule{},
		workloads:  WorkloadInstrumentRule{},
	}

	return setup
}
func (m *SetupManager) Start(context.Context) error {
	m.UpdateAnnotationsByRule()
	m.logger.Info("setup manager sync config done")
	m.cfg.WatchConfig()
	m.cfg.OnConfigChange(func(e fsnotify.Event) {
		m.UpdateAnnotationsByRule()
		m.logger.Info("setup manager sync config done")
	})
	return nil
}

func (m *SetupManager) UpdateAnnotationsByRule() {
	var instrumentAll, forceInstrumentAll bool
	instrumentAll = m.cfg.GetBool("instrument-all-namespace")
	forceInstrumentAll = m.cfg.GetBool("force-instrument-all-namespace")
	m.logger.Info("update annotations", "instrument-all-namespace", instrumentAll, "force-instrument-all-namespace", forceInstrumentAll)
	// 强制注入所有的NS和workload
	if forceInstrumentAll {
		// TODO deal with error
		// 对所有可访问的NS(跳过kube-system)添加注入标记
		nsList, _ := m.namespaces.InstrumentAll(m.logger, m.client)
		// 同时向所有可访问的workload添加注入标记
		for _, namespace := range nsList {
			// TODO deal with error
			_ = m.workloads.InstrumentAll(m.logger, m.client, namespace)
		}
	}

	// TODO deal with error
	// 对所有可访问的NS(跳过kube-system)添加注入标记
	nsList, err := m.namespaces.InstrumentWithCfg(m.logger, m.client, m.cfg, instrumentAll)
	if err != nil {
		m.logger.Error(err, "error list namespace")
	}
	// 同时向所有可访问的workload添加注入标记
	for _, namespace := range nsList {
		// TODO deal with error
		err := m.workloads.InstrumentWithCfg(m.logger, m.client, namespace, m.cfg, instrumentAll)
		if err != nil {
			m.logger.Error(err, "error instrument workload", "namespace", namespace)
		}
	}
}

type SetupRule interface {
	DisableList() []string
	EnableList() []string
}

func getJsonMergePatchForInstrumentationLabel(enabled bool) []byte {
	labelJsonMergePatchValue := "null"
	if enabled {
		labelJsonMergePatchValue = fmt.Sprintf("\"%s\"", consts.InstrumentationEnabled)
	} else {
		labelJsonMergePatchValue = fmt.Sprintf("\"%s\"", consts.InstrumentationDisabled)
	}

	jsonMergePatchContent := fmt.Sprintf(`{"metadata":{"labels":{"%s":%s}}}`, consts.OdigosInstrumentationLabel, labelJsonMergePatchValue)
	return []byte(jsonMergePatchContent)
}
