package layouts

import (
	"fmt"
	"strings"
)

type erupe921Profile struct{}

func Erupe921() Profile {
	return erupe921Profile{}
}

func (erupe921Profile) Name() string {
	return "9.2.1"
}

func (erupe921Profile) ReadyPath() string {
	return "/launcher"
}

func (erupe921Profile) PublicPort(cfg BaseConfig) (int, error) {
	if cfg.SignV2.Port <= 0 {
		return 0, fmt.Errorf("config.json has invalid SignV2.Port")
	}
	return cfg.SignV2.Port, nil
}

func (erupe921Profile) ResolveClientMode(cfg BaseConfig, fallback string) (string, error) {
	clientMode := strings.TrimSpace(cfg.ClientMode)
	if clientMode == "" {
		return "", fmt.Errorf("config.json is missing ClientMode")
	}
	return clientMode, nil
}

func (erupe921Profile) RewriteConfig(cfg map[string]any, upstreamPort int) error {
	signV2Value, ok := cfg["SignV2"].(map[string]any)
	if !ok {
		return fmt.Errorf("config.json is missing SignV2 object")
	}
	signV2Value["Enabled"] = true
	signV2Value["Port"] = upstreamPort
	cfg["SignV2"] = signV2Value
	return nil
}