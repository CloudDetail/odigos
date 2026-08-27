package setup

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/viper"
)

func TestRemoteNamespaceConfigHasPriorityOverLocalConfig(t *testing.T) {
	local := viper.New()
	local.Set("instrument-all-namespace", false)
	local.Set("force-instrument-all-namespace", true)
	local.Set("namespace.local", "enabledFuture")
	local.Set("workload.orders.deployment/api", "disabled")
	store := NewConfigStore(local, true)

	if store.InjectionAllowed() {
		t.Fatal("daemon-backed injection was allowed before the first sync")
	}
	store.SetRemote(NamespaceInjection{
		InstrumentAll:        true,
		InstrumentNS:         []string{"orders"},
		InstrumentDisabledNS: []string{"monitoring"},
		Namespaces: []NamespaceRule{
			{Namespace: "orders", Mode: "enabled"},
			{Namespace: "monitoring", Mode: "disabled"},
		},
		Workloads: []WorkloadInjection{{Namespace: "orders", Kind: "deployment", Name: "worker", Enabled: true}},
	})
	if !store.InjectionAllowed() || !store.GetBool("instrument-all-namespace") || store.GetBool("force-instrument-all-namespace") {
		t.Fatalf("remote global config was not applied")
	}
	if store.GetString("namespace.orders") != "enabled" || store.GetString("namespace.monitoring") != "disabled" || store.GetString("namespace.local") != "" {
		t.Fatalf("remote namespace config did not replace local namespace config: %+v", store.GetStringMap("namespace"))
	}
	orders, ok := store.GetStringMap("workload")["orders"].(map[string]any)
	if !ok || orders["deployment/worker"] != "enabled" || orders["deployment/api"] != nil {
		t.Fatal("remote workload config did not completely replace local config")
	}
	store.MarkDaemonUnavailable()
	if store.InjectionAllowed() {
		t.Fatal("injection remained enabled after daemon-go became unavailable")
	}
	if store.GetBool("force-instrument-all-namespace") || store.GetString("namespace.local") != "" {
		t.Fatal("local ConfigMap became effective while remote control was enabled")
	}
}

func TestEnabledNamespaceDoesNotEnableFutureWorkloads(t *testing.T) {
	if checkIfEnabled(true, "enabled", true) {
		t.Fatal("enabled namespace inherited instrument-all and enabled future workloads")
	}
	if !checkIfEnabled(true, "enabledFuture", false) {
		t.Fatal("enabledFuture namespace did not enable future workloads")
	}
}

func TestDaemonConfigClientPausesWhenDaemonHasNoRemotePolicy(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "odigos-namespace-config-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "config.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	local := viper.New()
	local.Set("instrument-all-namespace", true)
	store := NewConfigStore(local, true)
	var updates atomic.Int32
	client := NewDaemonConfigClient(socket, 10*time.Millisecond, 100*time.Millisecond, store, logr.Discard(), func() {
		updates.Add(1)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Start(ctx) }()
	go func() {
		odigletReady := true
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			var request daemonNamespaceConfigMessage
			_ = json.NewDecoder(conn).Decode(&request)
			_ = json.NewEncoder(conn).Encode(daemonNamespaceConfigMessage{
				Type:            "namespace_injection_config",
				ProtocolVersion: daemonNamespaceConfigProtocolVersion,
				OK:              true,
				OdigletReady:    &odigletReady,
			})
			_ = conn.Close()
		}
	}()

	time.Sleep(50 * time.Millisecond)
	if store.InjectionAllowed() || updates.Load() != 0 {
		t.Fatal("missing remote policy did not pause injection")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDaemonConfigClientDoesNotTreatUnknownOdigletReadinessAsAllDown(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "odigos-namespace-config-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "config.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			var request daemonNamespaceConfigMessage
			_ = json.NewDecoder(conn).Decode(&request)
			_ = json.NewEncoder(conn).Encode(daemonNamespaceConfigMessage{
				Type:            "namespace_injection_config",
				ProtocolVersion: daemonNamespaceConfigProtocolVersion,
				OK:              true,
				Config:          &NamespaceInjection{},
			})
			_ = conn.Close()
		}
	}()

	store := NewConfigStore(viper.New(), true)
	var updates atomic.Int32
	client := NewDaemonConfigClient(socket, 10*time.Millisecond, 100*time.Millisecond, store, logr.Discard(), func() {
		updates.Add(1)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Start(ctx) }()
	waitForCondition(t, time.Second, func() bool { return store.InjectionAllowed() && updates.Load() == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDaemonConfigClientPausesAndRecovers(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "odigos-namespace-config-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "config.sock")
	store := NewConfigStore(viper.New(), true)
	var updates atomic.Int32
	client := NewDaemonConfigClient(socket, 10*time.Millisecond, 100*time.Millisecond, store, logr.Discard(), func() {
		updates.Add(1)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Start(ctx) }()

	time.Sleep(25 * time.Millisecond)
	if store.InjectionAllowed() {
		t.Fatal("injection was allowed while the daemon socket was absent")
	}

	listener, err := net.Listen("unix", socket)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	serverDone := make(chan struct{})
	odigletReady := true
	go func() {
		defer close(serverDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var request daemonNamespaceConfigMessage
			_ = json.NewDecoder(conn).Decode(&request)
			_ = json.NewEncoder(conn).Encode(daemonNamespaceConfigMessage{
				Type:            "namespace_injection_config",
				ProtocolVersion: daemonNamespaceConfigProtocolVersion,
				OK:              true,
				OdigletReady:    &odigletReady,
				Config:          &NamespaceInjection{InstrumentNS: []string{"orders"}},
			})
			_ = conn.Close()
		}
	}()
	waitForCondition(t, time.Second, func() bool { return store.InjectionAllowed() && updates.Load() == 1 })

	_ = listener.Close()
	<-serverDone
	_ = os.Remove(socket)
	waitForCondition(t, time.Second, func() bool { return !store.InjectionAllowed() })

	listener, err = net.Listen("unix", socket)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		var request daemonNamespaceConfigMessage
		_ = json.NewDecoder(conn).Decode(&request)
		_ = json.NewEncoder(conn).Encode(daemonNamespaceConfigMessage{
			Type:            "namespace_injection_config",
			ProtocolVersion: daemonNamespaceConfigProtocolVersion,
			OK:              true,
			OdigletReady:    &odigletReady,
			Config:          &NamespaceInjection{InstrumentNS: []string{"orders"}},
		})
	}()
	waitForCondition(t, time.Second, func() bool { return store.InjectionAllowed() && updates.Load() == 2 })
	_ = listener.Close()

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDaemonConfigClientPausesUntilAnOdigletIsReady(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "odigos-namespace-config-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "config.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	var odigletState atomic.Int32
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			var request daemonNamespaceConfigMessage
			_ = json.NewDecoder(conn).Decode(&request)
			state := odigletState.Load()
			var ready *bool
			if state != 1 {
				value := state == 2
				ready = &value
			}
			_ = json.NewEncoder(conn).Encode(daemonNamespaceConfigMessage{
				Type:            "namespace_injection_config",
				ProtocolVersion: daemonNamespaceConfigProtocolVersion,
				OK:              true,
				OdigletReady:    ready,
				Config:          &NamespaceInjection{},
			})
			_ = conn.Close()
		}
	}()

	store := NewConfigStore(viper.New(), true)
	var updates atomic.Int32
	client := NewDaemonConfigClient(socket, 10*time.Millisecond, 100*time.Millisecond, store, logr.Discard(), func() {
		updates.Add(1)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Start(ctx) }()
	time.Sleep(30 * time.Millisecond)
	if store.InjectionAllowed() || updates.Load() != 0 {
		t.Fatal("injection was allowed while all odiglets were unavailable")
	}

	odigletState.Store(1)
	time.Sleep(30 * time.Millisecond)
	if store.InjectionAllowed() || updates.Load() != 0 {
		t.Fatal("unknown readiness cleared an explicit all-down block")
	}

	odigletState.Store(2)
	waitForCondition(t, time.Second, func() bool { return store.InjectionAllowed() && updates.Load() == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
