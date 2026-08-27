package setup

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

const daemonNamespaceConfigProtocolVersion = 1

var namespaceNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
var workloadNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)
var environmentVariablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type daemonNamespaceConfigMessage struct {
	Type            string              `json:"type"`
	ProtocolVersion int                 `json:"protocolVersion,omitempty"`
	OK              bool                `json:"ok,omitempty"`
	OdigletReady    *bool               `json:"odigletReady,omitempty"`
	Error           string              `json:"error,omitempty"`
	Config          *NamespaceInjection `json:"config,omitempty"`
	Inventory       *WorkloadInventory  `json:"inventory,omitempty"`
}

type daemonNamespaceConfigSnapshot struct {
	config       *NamespaceInjection
	odigletReady *bool
}

type DaemonConfigClient struct {
	socketPath      string
	retryInterval   time.Duration
	requestTimeout  time.Duration
	store           *ConfigStore
	logger          logr.Logger
	onUpdate        func()
	inventorySource func(context.Context) (*WorkloadInventory, error)
}

func (c *DaemonConfigClient) SetInventorySource(source func(context.Context) (*WorkloadInventory, error)) {
	c.inventorySource = source
}

func NewDaemonConfigClient(socketPath string, retryInterval, requestTimeout time.Duration, store *ConfigStore, logger logr.Logger, onUpdate func()) *DaemonConfigClient {
	return &DaemonConfigClient{
		socketPath:     socketPath,
		retryInterval:  retryInterval,
		requestTimeout: requestTimeout,
		store:          store,
		logger:         logger,
		onUpdate:       onUpdate,
	}
}

func (c *DaemonConfigClient) Start(ctx context.Context) error {
	blockedByOdiglets := false
	for {
		snapshot, err := c.fetch(ctx)
		if err != nil {
			c.store.MarkDaemonUnavailable()
			c.logger.Error(err, "pause namespace injection because daemon-go config is unavailable", "retryInterval", c.retryInterval.String())
		} else if snapshot.odigletReady != nil && !*snapshot.odigletReady {
			blockedByOdiglets = true
			if c.store.MarkDaemonUnavailable() {
				c.logger.Info("pause namespace injection because no odiglet is running normally")
			}
		} else {
			if snapshot.odigletReady != nil {
				blockedByOdiglets = false
			}
			if !blockedByOdiglets {
				if snapshot.config == nil {
					c.store.MarkDaemonUnavailable()
					c.logger.Info("pause namespace injection because daemon-go returned no remote policy")
				} else if c.store.SetRemote(*snapshot.config) {
					config := snapshot.config
					c.logger.Info("namespace injection config synchronized from daemon-go", "instrumentAll", config.InstrumentAll, "enabledNamespaces", len(config.InstrumentNS), "disabledNamespaces", len(config.InstrumentDisabledNS))
					c.onUpdate()
				}
			}
		}

		timer := time.NewTimer(c.retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (c *DaemonConfigClient) fetch(ctx context.Context) (daemonNamespaceConfigSnapshot, error) {
	dialer := net.Dialer{Timeout: c.requestTimeout}
	conn, err := dialer.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return daemonNamespaceConfigSnapshot{}, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(c.requestTimeout)); err != nil {
		return daemonNamespaceConfigSnapshot{}, err
	}
	request := daemonNamespaceConfigMessage{Type: "get_namespace_injection_config", ProtocolVersion: daemonNamespaceConfigProtocolVersion}
	if c.inventorySource != nil {
		inventory, inventoryErr := c.inventorySource(ctx)
		if inventoryErr != nil {
			c.logger.Error(inventoryErr, "collect Kubernetes workload inventory; continue fetching remote injection config")
		} else {
			request.Inventory = inventory
		}
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return daemonNamespaceConfigSnapshot{}, err
	}
	var response daemonNamespaceConfigMessage
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&response); err != nil {
		return daemonNamespaceConfigSnapshot{}, err
	}
	if response.Type != "namespace_injection_config" || response.ProtocolVersion != daemonNamespaceConfigProtocolVersion {
		return daemonNamespaceConfigSnapshot{}, errors.New("invalid daemon-go namespace config response")
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "daemon-go namespace config is unavailable"
		}
		return daemonNamespaceConfigSnapshot{}, errors.New(response.Error)
	}
	if response.Config == nil {
		return daemonNamespaceConfigSnapshot{}, errors.New("daemon-go namespace config response has no remote config")
	}
	config, err := normalizeRemoteNamespaceInjection(*response.Config)
	if err != nil {
		return daemonNamespaceConfigSnapshot{}, err
	}
	return daemonNamespaceConfigSnapshot{config: &config, odigletReady: response.OdigletReady}, nil
}

func normalizeRemoteNamespaceInjection(config NamespaceInjection) (NamespaceInjection, error) {
	if len(config.InstrumentNS)+len(config.InstrumentDisabledNS)+len(config.Namespaces)+len(config.Workloads) > 10000 {
		return NamespaceInjection{}, errors.New("namespace injection config exceeds 10000 entries")
	}
	disabled, err := normalizeRemoteNamespaceList(config.InstrumentDisabledNS)
	if err != nil {
		return NamespaceInjection{}, fmt.Errorf("invalid disabled namespace: %w", err)
	}
	enabled, err := normalizeRemoteNamespaceList(config.InstrumentNS)
	if err != nil {
		return NamespaceInjection{}, fmt.Errorf("invalid enabled namespace: %w", err)
	}
	disabledSet := make(map[string]struct{}, len(disabled))
	for _, namespace := range disabled {
		disabledSet[namespace] = struct{}{}
	}
	filteredEnabled := enabled[:0]
	for _, namespace := range enabled {
		if _, disabled := disabledSet[namespace]; !disabled {
			filteredEnabled = append(filteredEnabled, namespace)
		}
	}
	workloads := make([]WorkloadInjection, 0, len(config.Workloads))
	seenWorkloads := make(map[string]struct{}, len(config.Workloads))
	for _, workload := range config.Workloads {
		workload.Namespace = strings.TrimSpace(workload.Namespace)
		workload.Kind = strings.ToLower(strings.TrimSpace(workload.Kind))
		workload.Name = strings.TrimSpace(workload.Name)
		if _, err := normalizeRemoteNamespaceList([]string{workload.Namespace}); err != nil {
			return NamespaceInjection{}, err
		}
		switch workload.Kind {
		case "deployment", "statefulset", "daemonset":
		default:
			return NamespaceInjection{}, fmt.Errorf("unsupported workload kind %q", workload.Kind)
		}
		if len(workload.Name) == 0 || len(workload.Name) > 253 || !workloadNamePattern.MatchString(workload.Name) {
			return NamespaceInjection{}, fmt.Errorf("invalid Kubernetes workload name %q", workload.Name)
		}
		key := workload.Namespace + "\x00" + workload.Kind + "\x00" + workload.Name
		if _, exists := seenWorkloads[key]; exists {
			return NamespaceInjection{}, fmt.Errorf("duplicate workload rule %s/%s/%s", workload.Namespace, workload.Kind, workload.Name)
		}
		seenWorkloads[key] = struct{}{}
		workloads = append(workloads, workload)
	}
	serviceName, err := normalizeRemoteServiceNamePolicy(config.ServiceName)
	if err != nil {
		return NamespaceInjection{}, err
	}
	namespaceRules := make([]NamespaceRule, 0, len(config.Namespaces))
	seenNamespaceRules := make(map[string]struct{}, len(config.Namespaces))
	for _, rule := range config.Namespaces {
		rule.Namespace = strings.TrimSpace(rule.Namespace)
		rule.Mode = strings.TrimSpace(rule.Mode)
		if _, err := normalizeRemoteNamespaceList([]string{rule.Namespace}); err != nil {
			return NamespaceInjection{}, err
		}
		switch rule.Mode {
		case "disabled", "enabled", "enabledFuture":
		default:
			return NamespaceInjection{}, fmt.Errorf("unsupported namespace injection mode %q", rule.Mode)
		}
		if _, exists := seenNamespaceRules[rule.Namespace]; exists {
			return NamespaceInjection{}, fmt.Errorf("duplicate namespace rule %q", rule.Namespace)
		}
		seenNamespaceRules[rule.Namespace] = struct{}{}
		namespaceRules = append(namespaceRules, rule)
	}
	return NamespaceInjection{
		InstrumentAll: config.InstrumentAll, InstrumentNS: filteredEnabled,
		InstrumentDisabledNS: disabled, Namespaces: namespaceRules,
		Workloads: workloads, ServiceName: serviceName,
	}, nil
}

func normalizeRemoteServiceNamePolicy(policy *ServiceNamePolicy) (*ServiceNamePolicy, error) {
	if policy == nil {
		return nil, nil
	}
	if len(policy.Mappings) > 1000 || len(policy.Variables) > 128 || len(policy.DefaultFormat) > 1024 {
		return nil, errors.New("service name policy exceeds supported limits")
	}
	result := &ServiceNamePolicy{DefaultFormat: strings.TrimSpace(policy.DefaultFormat)}
	seenVariables := make(map[string]struct{}, len(policy.Variables))
	for _, variable := range policy.Variables {
		variable.Name = strings.TrimSpace(variable.Name)
		variable.Source = strings.TrimSpace(variable.Source)
		if !environmentVariablePattern.MatchString(variable.Name) {
			return nil, fmt.Errorf("invalid service name variable %q", variable.Name)
		}
		switch variable.Source {
		case "workloadName", "workflowName", "deployName", "containerName":
		default:
			return nil, fmt.Errorf("unsupported service name source %q", variable.Source)
		}
		if len(variable.Regex) > 1024 || len(variable.Replacement) > 1024 {
			return nil, fmt.Errorf("service name variable %q exceeds supported limits", variable.Name)
		}
		if variable.Regex != "" {
			if _, err := regexp.Compile(variable.Regex); err != nil {
				return nil, fmt.Errorf("invalid service name regex for %q: %w", variable.Name, err)
			}
		}
		if _, exists := seenVariables[variable.Name]; exists {
			return nil, fmt.Errorf("duplicate service name variable %q", variable.Name)
		}
		seenVariables[variable.Name] = struct{}{}
		result.Variables = append(result.Variables, variable)
	}
	for _, mapping := range policy.Mappings {
		mapping.NamespacePattern = strings.TrimSpace(mapping.NamespacePattern)
		mapping.WorkloadPattern = strings.TrimSpace(mapping.WorkloadPattern)
		mapping.ServiceName = strings.TrimSpace(mapping.ServiceName)
		if mapping.NamespacePattern == "" || mapping.WorkloadPattern == "" || mapping.ServiceName == "" || len(mapping.ServiceName) > 255 {
			return nil, errors.New("service name mapping requires namespace pattern, workload pattern, and service name")
		}
		if _, err := regexp.Compile(mapping.NamespacePattern); err != nil {
			return nil, fmt.Errorf("invalid service name namespace regex %q: %w", mapping.NamespacePattern, err)
		}
		if _, err := regexp.Compile(mapping.WorkloadPattern); err != nil {
			return nil, fmt.Errorf("invalid service name workload regex %q: %w", mapping.WorkloadPattern, err)
		}
		result.Mappings = append(result.Mappings, mapping)
	}
	return result, nil
}

func normalizeRemoteNamespaceList(namespaces []string) ([]string, error) {
	seen := make(map[string]struct{}, len(namespaces))
	result := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		namespace = strings.TrimSpace(namespace)
		if len(namespace) == 0 || len(namespace) > 63 || !namespaceNamePattern.MatchString(namespace) {
			return nil, fmt.Errorf("invalid Kubernetes namespace %q", namespace)
		}
		if _, exists := seen[namespace]; exists {
			continue
		}
		seen[namespace] = struct{}{}
		result = append(result, namespace)
	}
	return result, nil
}
