package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	layouts "lilbutter-wrapper/Layouts"
)

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadWrapperConfigPrefersMezeportaFileAndKeys(t *testing.T) {
	root := t.TempDir()

	writeJSONFile(t, filepath.Join(root, legacyWrapperConfigFileName), map[string]any{
		"erupe_version":      "9.2",
		"9.2ClientMode":      "FW.5",
		"MaxClientPatch":     5,
		"AltClientSaveCache": false,
		"AltClientLogs":      false,
		"EnableBinCustom":    false,
	})
	writeJSONFile(t, filepath.Join(root, wrapperConfigFileName), map[string]any{
		"erupe_version":       "9.3+",
		"9.2ClientMode":       "ZZ",
		"MaxClientPatch":      2,
		"SaveCacheFetch":      true,
		"MailFetch":           false,
		"DistributionFetch":   false,
		"ClientImagesHosting": false,
		"MezeportaLogs":       true,
		"EnableBinCustom":     true,
	})

	cfg, err := loadWrapperConfig(root)
	if err != nil {
		t.Fatalf("loadWrapperConfig returned error: %v", err)
	}

	if cfg.ErupeVersion != "9.3+" {
		t.Fatalf("ErupeVersion = %q, want %q", cfg.ErupeVersion, "9.3+")
	}
	if cfg.ClientMode92 != "ZZ" {
		t.Fatalf("ClientMode92 = %q, want %q", cfg.ClientMode92, "ZZ")
	}
	if cfg.MaxClientPatch != 2 {
		t.Fatalf("MaxClientPatch = %d, want %d", cfg.MaxClientPatch, 2)
	}
	if !cfg.SaveCacheFetch {
		t.Fatal("SaveCacheFetch = false, want true")
	}
	if cfg.MailFetch {
		t.Fatal("MailFetch = true, want false")
	}
	if cfg.DistributionFetch {
		t.Fatal("DistributionFetch = true, want false")
	}
	if cfg.ClientImagesHosting {
		t.Fatal("ClientImagesHosting = true, want false")
	}
	if !cfg.MezeportaLogs {
		t.Fatal("MezeportaLogs = false, want true")
	}
	if !cfg.EnableBinCustom {
		t.Fatal("EnableBinCustom = false, want true")
	}
}

func TestLoadWrapperConfigFallsBackToLegacyFileAndKeys(t *testing.T) {
	root := t.TempDir()

	writeJSONFile(t, filepath.Join(root, legacyWrapperConfigFileName), map[string]any{
		"erupe_version":      "9.2.1",
		"9.2ClientMode":      "G10",
		"MaxClientPatch":     4,
		"AltClientSaveCache": false,
		"AltClientLogs":      false,
		"EnableBinCustom":    true,
	})

	cfg, err := loadWrapperConfig(root)
	if err != nil {
		t.Fatalf("loadWrapperConfig returned error: %v", err)
	}

	if cfg.ErupeVersion != "9.2.1" {
		t.Fatalf("ErupeVersion = %q, want %q", cfg.ErupeVersion, "9.2.1")
	}
	if cfg.ClientMode92 != "G10" {
		t.Fatalf("ClientMode92 = %q, want %q", cfg.ClientMode92, "G10")
	}
	if cfg.MaxClientPatch != 4 {
		t.Fatalf("MaxClientPatch = %d, want %d", cfg.MaxClientPatch, 4)
	}
	if cfg.SaveCacheFetch {
		t.Fatal("SaveCacheFetch = true, want false")
	}
	if !cfg.MailFetch {
		t.Fatal("MailFetch = false, want true")
	}
	if !cfg.DistributionFetch {
		t.Fatal("DistributionFetch = false, want true")
	}
	if !cfg.ClientImagesHosting {
		t.Fatal("ClientImagesHosting = false, want true")
	}
	if cfg.MezeportaLogs {
		t.Fatal("MezeportaLogs = true, want false")
	}
	if !cfg.EnableBinCustom {
		t.Fatal("EnableBinCustom = false, want true")
	}
}

func TestWriteDerivedUpstreamConfigIncludesEnableBinCustom(t *testing.T) {
	root := t.TempDir()

	writeJSONFile(t, filepath.Join(root, baseConfigFileName), map[string]any{
		"BinPath": "bin",
		"API": map[string]any{
			"Enabled": true,
			"Port":    8080,
		},
	})

	wrapperCfg := defaultWrapperConfig()
	wrapperCfg.EnableBinCustom = true

	tmpDir, cleanup, err := writeDerivedUpstreamConfig(root, layouts.Modern(), wrapperCfg, 18081)
	if err != nil {
		t.Fatalf("writeDerivedUpstreamConfig returned error: %v", err)
	}
	defer cleanup()

	raw, err := os.ReadFile(filepath.Join(tmpDir, baseConfigFileName))
	if err != nil {
		t.Fatalf("read derived config: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal derived config: %v", err)
	}

	if enabled, ok := cfg["EnableBinCustom"].(bool); !ok || !enabled {
		t.Fatalf("EnableBinCustom = %#v, want true", cfg["EnableBinCustom"])
	}

	wantBinPath := filepath.Join(root, "bin")
	if got, ok := cfg["BinPath"].(string); !ok || got != wantBinPath {
		t.Fatalf("BinPath = %#v, want %q", cfg["BinPath"], wantBinPath)
	}
}
