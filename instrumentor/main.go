/*
Copyright 2022.

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

package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/odigos-io/odigos/common/consts"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/labels"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/go-logr/zapr"
	bridge "github.com/odigos-io/opentelemetry-zap-bridge"

	v1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"

	"github.com/odigos-io/odigos/instrumentor/controllers/deleteinstrumentedapplication"
	"github.com/odigos-io/odigos/instrumentor/controllers/instrumentationdevice"
	"github.com/odigos-io/odigos/instrumentor/setup"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	//+kubebuilder:scaffold:imports

	_ "net/http/pprof"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(v1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var telemetryDisabled bool
	var namespaceConfigFromDaemon bool
	var daemonNamespaceConfigSocket string
	var daemonNamespaceConfigRetryInterval time.Duration
	var daemonNamespaceConfigRequestTimeout time.Duration

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&telemetryDisabled, "telemetry-disabled", false, "Disable telemetry")
	flag.BoolVar(&namespaceConfigFromDaemon, "namespace-config-from-daemon", false, "Use daemon-go as the namespace injection config source")
	flag.StringVar(&daemonNamespaceConfigSocket, "daemon-namespace-config-socket", "/var/run/odigos-config/namespace-config.sock", "daemon-go namespace injection config Unix socket")
	flag.DurationVar(&daemonNamespaceConfigRetryInterval, "daemon-namespace-config-retry-interval", 15*time.Second, "daemon-go namespace config retry interval")
	flag.DurationVar(&daemonNamespaceConfigRequestTimeout, "daemon-namespace-config-request-timeout", 5*time.Second, "daemon-go namespace config request timeout")

	opts := ctrlzap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	zapLogger := ctrlzap.NewRaw(ctrlzap.UseFlagOptions(&opts))
	zapLogger = bridge.AttachToZapLogger(zapLogger)
	logger := zapr.NewLogger(zapLogger)
	ctrl.SetLogger(logger)

	instrumentedSelector := labels.Set{consts.OdigosInstrumentationLabel: consts.InstrumentationEnabled}.AsSelector()
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "201bdfa0.odigos.io",
		Cache: cache.Options{
			DefaultTransform: cache.TransformStripManagedFields(),
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Namespace{}: {
					Label: instrumentedSelector,
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupMgr, err := createSetupManager(namespaceConfigFromDaemon)
	if err == nil {
		mgr.Add(setupMgr)
	} else {
		setupLog.Error(err, "unable to create setup manager")
		os.Exit(1)
	}

	if setupMgr != nil {
		err = instrumentationdevice.SetupWithManager(mgr, setupMgr.Cfg)
	} else {
		err = instrumentationdevice.SetupWithManager(mgr, nil)
	}

	if err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}
	if namespaceConfigFromDaemon {
		if !filepath.IsAbs(daemonNamespaceConfigSocket) {
			setupLog.Error(errors.New("socket path must be absolute"), "invalid daemon namespace config socket", "path", daemonNamespaceConfigSocket)
			os.Exit(1)
		}
		if daemonNamespaceConfigRetryInterval <= 0 || daemonNamespaceConfigRequestTimeout <= 0 {
			setupLog.Error(errors.New("retry interval and request timeout must be positive"), "invalid daemon namespace config timing")
			os.Exit(1)
		}
		store := setupMgr.Cfg.(*setup.ConfigStore)
		daemonConfigClient := setup.NewDaemonConfigClient(
			daemonNamespaceConfigSocket,
			daemonNamespaceConfigRetryInterval,
			daemonNamespaceConfigRequestTimeout,
			store,
			setupLog,
			setupMgr.UpdateAnnotationsByRule,
		)
		daemonConfigClient.SetInventorySource(setupMgr.WorkloadInventory)
		if err := mgr.Add(daemonConfigClient); err != nil {
			setupLog.Error(err, "unable to add daemon namespace config client")
			os.Exit(1)
		}
	}

	err = deleteinstrumentedapplication.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	go common.StartPprofServer(setupLog)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func createSetupManager(namespaceConfigFromDaemon bool) (*setup.SetupManager, error) {
	path, find := os.LookupEnv("INSTRUMENT_CONFIGS_PATH")
	if !find {
		path = "config/instrument-conf.yaml"
	}

	// 读取setup配置
	setupCfg := viper.New()
	setupCfg.SetConfigFile(path)
	err := setupCfg.ReadInConfig()
	if err != nil {
		return nil, err
	}

	// 创建新的k8s client
	k8sCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	client, err := client.NewWithWatch(k8sCfg, client.Options{
		Scheme: runtime.NewScheme(),
	})
	if err != nil {
		return nil, err
	}

	if err := corev1.AddToScheme(client.Scheme()); err != nil {
		return nil, err
	}

	if err := appsv1.AddToScheme(client.Scheme()); err != nil {
		return nil, err
	}

	configStore := setup.NewConfigStore(setupCfg, namespaceConfigFromDaemon)
	smgr := setup.NewSetupManager(setupLog, configStore, client)
	return smgr, nil
}
