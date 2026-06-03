package layouts

import (
	"fmt"
	"strings"
)

type erupe92Profile struct{}

func Erupe92() Profile {
	return erupe92Profile{}
}

func (erupe92Profile) Name() string {
	return "9.2"
}

func (erupe92Profile) ReadyPath() string {
	return "/launcher"
}

func (erupe92Profile) PublicPort(cfg BaseConfig) (int, error) {
	if cfg.SignV2.Port <= 0 {
		return 0, fmt.Errorf("config.json has invalid SignV2.Port")
	}
	return cfg.SignV2.Port, nil
}

func (erupe92Profile) ResolveClientMode(cfg BaseConfig, fallback string) (string, error) {
	clientMode := strings.TrimSpace(cfg.ClientMode)
	if clientMode != "" {
		return clientMode, nil
	}

	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return fallback, nil
	}

	return "", fmt.Errorf("config.json is missing ClientMode and 9.2ClientMode is blank")
}

func (erupe92Profile) RewriteConfig(cfg map[string]any, upstreamPort int) error {
	signV2Value, ok := cfg["SignV2"].(map[string]any)
	if !ok {
		return fmt.Errorf("config.json is missing SignV2 object")
	}
	signV2Value["Enabled"] = true
	signV2Value["Port"] = upstreamPort
	cfg["SignV2"] = signV2Value
	return nil
}