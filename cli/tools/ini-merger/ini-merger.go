package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/ini.v1"
)

func main() {
	if len(os.Args) == 2 {
		fillMergeConfigWithEnv()
	} else if len(os.Args) == 3 {
		mergeConfig()
	} else {
		fmt.Println("Usage: ini-merger <new-config> <old-config>\n  or\n  ini-merger <config>")
		os.Exit(1)
	}
}

func mergeConfig() {
	newCfg, err := ini.Load(os.Args[1])
	if err != nil {
		fmt.Printf("无法读取 %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	oldCfg, err := ini.Load(os.Args[2])
	if err != nil {
		fmt.Printf("无法读取 %s: %v\n", os.Args[2], err)
		os.Exit(1)
	}

	for _, section := range newCfg.Sections() {
		oldCfg.DeleteSection(section.Name())
		emptySec, err := oldCfg.NewSection(section.Name())
		if err != nil {
			fmt.Printf("无法合并section %s: %v\n", section.Name(), err)
			os.Exit(1)
		}
		for _, key := range section.Keys() {
			emptySec.Key(key.Name()).SetValue(key.Value())
		}
	}

	if oldCfg.HasSection(ini.DefaultSection) {
		oldCfg.DeleteSection(ini.DefaultSection)
	}

	err = oldCfg.SaveTo(os.Args[2])
	if err != nil {
		fmt.Printf("无法保存新配置到 %s: %v\n", os.Args[2], err)
		os.Exit(1)
	}
}

func fillMergeConfigWithEnv() {
	cfg, err := ini.Load(os.Args[1])
	if err != nil {
		fmt.Printf("无法读取 %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	defaultEnv := GetDefaultInternalValue()

	for _, section := range cfg.Sections() {
		for _, key := range section.Keys() {
			rawValue := strings.TrimSpace(key.Value())
			if strings.HasPrefix(rawValue, "{{") && strings.HasSuffix(rawValue, "}}") {
				val := strings.TrimPrefix(rawValue, "{{")
				val = strings.TrimSuffix(val, "}}")

				envVal, find := os.LookupEnv(strings.TrimSpace(val))
				if find {
					key.SetValue(envVal)
				} else if v, find := defaultEnv[strings.TrimSpace(val)]; find {
					key.SetValue(v)
				}
			}
		}
	}

	if cfg.HasSection(ini.DefaultSection) {
		cfg.DeleteSection(ini.DefaultSection)
	}

	err = cfg.SaveTo(os.Args[1])
	if err != nil {
		fmt.Printf("无法保存新配置到 %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
}

func GetDefaultInternalValue() map[string]string {
	val, find := os.LookupEnv("MY_NODE_IP")
	if !find {
		val = "localhost"
	}

	return map[string]string{
		"OTEL_EXPORTER_GRPC_ENDPOINT":      fmt.Sprintf("http://%s:%d", val, 4317),
		"OTEL_EXPORTER_HTTP_ENDPOINT":      fmt.Sprintf("http://%s:%d", val, 4318),
		"OTEL_EXPORTER_SKYWALING_ENDPOINT": fmt.Sprintf("http://%s:%d", val, 11800),
	}
}
