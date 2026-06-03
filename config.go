package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	layouts "lilbutter-wrapper/Layouts"
)

const (
	baseConfigFileName            = "config.json"
	mezeportaConfigFileName       = "Mezeporta.json"
	legacyAltClientConfigFileName = "AltClient.json"
	wrapperConfigFileName         = "MezeportaWrapper.json"
	legacyWrapperConfigFileName   = "AltClientWrapper.json"
	clientImagesDirName           = "ClientImages"
	gamePatchDirName              = "GamePatch"
	defaultUpstreamAPIPort        = 18080
	defaultPatchConcurrency       = 1
)

func upstreamExecutableNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"erupe-ce.exe", "erupe-ce"}
	}
	return []string{"erupe-ce"}
}

type baseConfig = layouts.BaseConfig

type wrapperConfig struct {
	ErupeVersion        string `json:"erupe_version"`
	ClientMode92        string `json:"9.2ClientMode"`
	MaxClientPatch      int    `json:"MaxClientPatch"`
	SaveCacheFetch      bool   `json:"SaveCacheFetch"`
	MailFetch           bool   `json:"MailFetch"`
	DistributionFetch   bool   `json:"DistributionFetch"`
	ClientImagesHosting bool   `json:"ClientImagesHosting"`
	MezeportaLogs       bool   `json:"MezeportaLogs"`
	EnableBinCustom     bool   `json:"EnableBinCustom"`
}

func defaultWrapperConfig() wrapperConfig {
	return wrapperConfig{
		ErupeVersion:        "9.3+",
		ClientMode92:        "",
		MaxClientPatch:      defaultPatchConcurrency,
		SaveCacheFetch:      true,
		MailFetch:           true,
		DistributionFetch:   true,
		ClientImagesHosting: true,
		MezeportaLogs:       true,
		EnableBinCustom:     false,
	}
}

type rawWrapperConfig struct {
	ErupeVersion        string `json:"erupe_version"`
	ClientMode92        string `json:"9.2ClientMode"`
	MaxClientPatch      int    `json:"MaxClientPatch"`
	SaveCacheFetch      *bool  `json:"SaveCacheFetch"`
	MezeportaSaveCache  *bool  `json:"MezeportaSaveCache"`
	AltClientSaveCache  *bool  `json:"AltClientSaveCache"`
	MailFetch           *bool  `json:"MailFetch"`
	DistributionFetch   *bool  `json:"DistributionFetch"`
	ClientImagesHosting *bool  `json:"ClientImagesHosting"`
	MezeportaLogs       *bool  `json:"MezeportaLogs"`
	AltClientLogs       *bool  `json:"AltClientLogs"`
	EnableBinCustom     *bool  `json:"EnableBinCustom"`
}

func currentServerRoot() (string, error) {
	return os.Getwd()
}

func resolveWrapperProfile(cfg wrapperConfig) (layouts.Profile, error) {
	return layouts.ResolveProfile(cfg.ErupeVersion)
}

func loadBaseConfig(root string) (baseConfig, error) {
	var cfg baseConfig
	configPath := filepath.Join(root, baseConfigFileName)
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, err
	}
	cfg, err = layouts.ParseBaseConfig(raw)
	if err != nil {
		return cfg, fmt.Errorf("parse %s: %w", configPath, err)
	}
	if strings.TrimSpace(cfg.Database.Host) == "" || cfg.Database.Port <= 0 || strings.TrimSpace(cfg.Database.Database) == "" {
		return cfg, fmt.Errorf("%s has invalid Database configuration", configPath)
	}
	return cfg, nil
}

func resolveConfigPath(root string, preferred string, fallback string) (string, error) {
	for _, name := range []string{preferred, fallback} {
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		switch {
		case err == nil:
			if info.IsDir() {
				return "", fmt.Errorf("%s is a directory", path)
			}
			return path, nil
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			return "", err
		}
	}

	return "", nil
}

func loadWrapperConfig(root string) (wrapperConfig, error) {
	cfg := defaultWrapperConfig()
	configPath, err := resolveConfigPath(root, wrapperConfigFileName, legacyWrapperConfigFileName)
	if err != nil {
		return cfg, err
	}
	if configPath == "" {
		return cfg, nil
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, err
	}

	var rawCfg rawWrapperConfig
	if err := json.Unmarshal(raw, &rawCfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", configPath, err)
	}

	cfg.ErupeVersion = rawCfg.ErupeVersion
	cfg.ClientMode92 = rawCfg.ClientMode92
	cfg.MaxClientPatch = rawCfg.MaxClientPatch
	switch {
	case rawCfg.SaveCacheFetch != nil:
		cfg.SaveCacheFetch = *rawCfg.SaveCacheFetch
	case rawCfg.MezeportaSaveCache != nil:
		cfg.SaveCacheFetch = *rawCfg.MezeportaSaveCache
	case rawCfg.AltClientSaveCache != nil:
		cfg.SaveCacheFetch = *rawCfg.AltClientSaveCache
	}
	if rawCfg.MailFetch != nil {
		cfg.MailFetch = *rawCfg.MailFetch
	}
	if rawCfg.DistributionFetch != nil {
		cfg.DistributionFetch = *rawCfg.DistributionFetch
	}
	if rawCfg.ClientImagesHosting != nil {
		cfg.ClientImagesHosting = *rawCfg.ClientImagesHosting
	}
	switch {
	case rawCfg.MezeportaLogs != nil:
		cfg.MezeportaLogs = *rawCfg.MezeportaLogs
	case rawCfg.AltClientLogs != nil:
		cfg.MezeportaLogs = *rawCfg.AltClientLogs
	}
	if rawCfg.EnableBinCustom != nil {
		cfg.EnableBinCustom = *rawCfg.EnableBinCustom
	}

	cfg.ErupeVersion = strings.TrimSpace(cfg.ErupeVersion)
	if cfg.ErupeVersion == "" {
		cfg.ErupeVersion = "9.3+"
	}
	cfg.ClientMode92 = strings.TrimSpace(cfg.ClientMode92)
	if cfg.MaxClientPatch <= 0 {
		cfg.MaxClientPatch = defaultPatchConcurrency
	}
	return cfg, nil
}

func resolveUpstreamAPIPort(publicPort int) int {
	if publicPort == defaultUpstreamAPIPort {
		return defaultUpstreamAPIPort + 1
	}
	return defaultUpstreamAPIPort
}

func makeConfigPathAbsolute(root string, raw any) any {
	value, ok := raw.(string)
	if !ok {
		return raw
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return raw
	}
	return filepath.Join(root, trimmed)
}

func writeDerivedUpstreamConfig(root string, profile layouts.Profile, wrapperCfg wrapperConfig, upstreamPort int) (string, func(), error) {
	configPath := filepath.Join(root, baseConfigFileName)
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return "", nil, err
	}

	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", nil, fmt.Errorf("parse %s: %w", configPath, err)
	}

	if binPath, ok := cfg["BinPath"]; ok {
		cfg["BinPath"] = makeConfigPathAbsolute(root, binPath)
	}
	cfg["EnableBinCustom"] = wrapperCfg.EnableBinCustom
	for _, key := range []string{"SaveDumps", "Screenshots", "Capture", "DevModeOptions"} {
		section, ok := cfg[key].(map[string]any)
		if !ok {
			continue
		}
		if outputDir, exists := section["OutputDir"]; exists {
			section["OutputDir"] = makeConfigPathAbsolute(root, outputDir)
		}
		if nestedSaveDumps, exists := section["SaveDumps"].(map[string]any); exists {
			if outputDir, hasOutputDir := nestedSaveDumps["OutputDir"]; hasOutputDir {
				nestedSaveDumps["OutputDir"] = makeConfigPathAbsolute(root, outputDir)
			}
			section["SaveDumps"] = nestedSaveDumps
		}
		cfg[key] = section
	}

	if err := profile.RewriteConfig(cfg, upstreamPort); err != nil {
		return "", nil, err
	}

	tmpDir, err := os.MkdirTemp("", "mezeporta-wrapper-*")
	if err != nil {
		return "", nil, err
	}
	tmpConfigPath := filepath.Join(tmpDir, baseConfigFileName)
	file, err := os.Create(tmpConfigPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	encodeErr := enc.Encode(cfg)
	closeErr := file.Close()
	if encodeErr != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, encodeErr
	}
	if closeErr != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, closeErr
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}
	return tmpDir, cleanup, nil
}

func resolvePatchRoot(root string, clientMode string) (string, error) {
	mode := strings.ToUpper(strings.TrimSpace(clientMode))
	if mode == "" {
		return "", fmt.Errorf("blank client mode")
	}

	group := ""
	switch {
	case strings.HasPrefix(mode, "FW"):
		group = "2-Forward"
	case strings.HasPrefix(mode, "S"):
		group = "1-Online"
	case strings.HasPrefix(mode, "G"):
		group = "3-G"
	case strings.HasPrefix(mode, "Z"):
		group = "4-Z"
	default:
		return "", fmt.Errorf("unsupported client mode %q", mode)
	}

	return filepath.Join(root, gamePatchDirName, group, mode), nil
}
