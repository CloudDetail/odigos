package setup

import (
	"reflect"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/odigos-io/odigos/common/envOverwrite"
	"github.com/spf13/viper"
)

type ConfigReader interface {
	GetBool(string) bool
	GetString(string) string
	GetStringMap(string) map[string]any
	InjectionAllowed() bool
}

type NamespaceInjection struct {
	InstrumentAll        bool                `json:"instrumentAll"`
	InstrumentNS         []string            `json:"instrumentNS"`
	InstrumentDisabledNS []string            `json:"instrumentDisabledNS"`
	Namespaces           []NamespaceRule     `json:"namespaces,omitempty"`
	Workloads            []WorkloadInjection `json:"workloads,omitempty"`
	ServiceName          *ServiceNamePolicy  `json:"serviceName,omitempty"`
}

type NamespaceRule struct {
	Namespace string `json:"namespace"`
	Mode      string `json:"mode"`
}

type WorkloadInjection struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
}

type ServiceNamePolicy struct {
	DefaultFormat string                `json:"defaultFormat"`
	Variables     []ServiceNameVariable `json:"variables"`
	Mappings      []ServiceNameMapping  `json:"mappings"`
}

type ServiceNameMapping struct {
	NamespacePattern string `json:"namespacePattern"`
	WorkloadPattern  string `json:"workloadPattern"`
	ServiceName      string `json:"serviceName"`
}

type ServiceNameVariable struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Regex       string `json:"regex"`
	Replacement string `json:"replacement"`
}

type ConfigStore struct {
	local         *viper.Viper
	requireDaemon bool

	mu        sync.RWMutex
	remote    *NamespaceInjection
	available bool
}

func NewConfigStore(local *viper.Viper, requireDaemon bool) *ConfigStore {
	if requireDaemon {
		envOverwrite.SetServiceNamePolicy(&envOverwrite.ServiceNamePolicy{})
	}
	return &ConfigStore{local: local, requireDaemon: requireDaemon, available: !requireDaemon}
}

func (s *ConfigStore) InjectionAllowed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.requireDaemon || s.available
}

func (s *ConfigStore) SetRemote(config NamespaceInjection) bool {
	copy := config
	copy.InstrumentNS = append([]string(nil), config.InstrumentNS...)
	copy.InstrumentDisabledNS = append([]string(nil), config.InstrumentDisabledNS...)
	copy.Namespaces = append([]NamespaceRule(nil), config.Namespaces...)
	copy.Workloads = append([]WorkloadInjection(nil), config.Workloads...)
	copy.ServiceName = cloneServiceNamePolicy(config.ServiceName)

	s.mu.Lock()
	changed := !s.available || s.remote == nil || !reflect.DeepEqual(*s.remote, copy)
	s.remote = &copy
	s.available = true
	s.mu.Unlock()
	envOverwrite.SetServiceNamePolicy(toEnvOverwritePolicy(copy.ServiceName))
	return changed
}

func (s *ConfigStore) MarkDaemonUnavailable() bool {
	s.mu.Lock()
	changed := s.available
	if s.requireDaemon {
		s.available = false
		s.remote = nil
	}
	s.mu.Unlock()
	if s.requireDaemon {
		envOverwrite.SetServiceNamePolicy(&envOverwrite.ServiceNamePolicy{})
	}
	return changed
}

func cloneServiceNamePolicy(policy *ServiceNamePolicy) *ServiceNamePolicy {
	if policy == nil {
		return nil
	}
	copy := *policy
	copy.Variables = append([]ServiceNameVariable(nil), policy.Variables...)
	copy.Mappings = append([]ServiceNameMapping(nil), policy.Mappings...)
	return &copy
}

func toEnvOverwritePolicy(policy *ServiceNamePolicy) *envOverwrite.ServiceNamePolicy {
	result := &envOverwrite.ServiceNamePolicy{}
	if policy == nil {
		return result
	}
	result.DefaultFormat = policy.DefaultFormat
	for _, variable := range policy.Variables {
		result.Variables = append(result.Variables, envOverwrite.ServiceNameVariable{
			Name: variable.Name, Source: variable.Source, Regex: variable.Regex, Replacement: variable.Replacement,
		})
	}
	for _, mapping := range policy.Mappings {
		result.Mappings = append(result.Mappings, envOverwrite.ServiceNameMapping{
			NamespacePattern: mapping.NamespacePattern,
			WorkloadPattern:  mapping.WorkloadPattern,
			ServiceName:      mapping.ServiceName,
		})
	}
	return result
}

func (s *ConfigStore) GetBool(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.requireDaemon && s.remote == nil {
		return false
	}
	if s.remote != nil {
		switch key {
		case "instrument-all-namespace":
			return s.remote.InstrumentAll
		case "force-instrument-all-namespace":
			return false
		}
	}
	return s.local.GetBool(key)
}

func (s *ConfigStore) GetString(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.requireDaemon && s.remote == nil {
		return ""
	}
	if s.remote != nil {
		if namespace, found := strings.CutPrefix(key, "namespace."); found && namespace != "" {
			if len(s.remote.Namespaces) > 0 {
				for _, rule := range s.remote.Namespaces {
					if rule.Namespace == namespace {
						return rule.Mode
					}
				}
				return ""
			}
			for _, disabled := range s.remote.InstrumentDisabledNS {
				if disabled == namespace {
					return "disabled"
				}
			}
			for _, enabled := range s.remote.InstrumentNS {
				if enabled == namespace {
					return "enabledFuture"
				}
			}
			return ""
		}
	}
	return s.local.GetString(key)
}

func (s *ConfigStore) GetStringMap(key string) map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.requireDaemon && s.remote == nil {
		return map[string]any{}
	}
	if key == "namespace" && s.remote != nil {
		if len(s.remote.Namespaces) > 0 {
			result := make(map[string]any, len(s.remote.Namespaces))
			for _, rule := range s.remote.Namespaces {
				result[rule.Namespace] = rule.Mode
			}
			return result
		}
		result := make(map[string]any, len(s.remote.InstrumentNS)+len(s.remote.InstrumentDisabledNS))
		for _, namespace := range s.remote.InstrumentNS {
			result[namespace] = "enabledFuture"
		}
		for _, namespace := range s.remote.InstrumentDisabledNS {
			result[namespace] = "disabled"
		}
		return result
	}
	if key == "workload" && s.remote != nil {
		result := make(map[string]any)
		for _, workload := range s.remote.Workloads {
			namespaceRules, ok := result[workload.Namespace].(map[string]any)
			if !ok {
				namespaceRules = make(map[string]any)
				result[workload.Namespace] = namespaceRules
			}
			operation := "disabled"
			if workload.Enabled {
				operation = "enabled"
			}
			namespaceRules[workload.Kind+"/"+workload.Name] = operation
		}
		return result
	}
	return s.local.GetStringMap(key)
}

func (s *ConfigStore) WatchLocalConfig(onChange func(fsnotify.Event)) {
	s.local.WatchConfig()
	s.local.OnConfigChange(onChange)
}
