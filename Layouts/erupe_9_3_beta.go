package layouts

import (
	"fmt"
	"strings"
)

// pre-v2 HTTP route names (/launcher, /login, /character/create).
type erupe93BetaProfile struct{}

func Erupe93Beta() Profile {
	return erupe93BetaProfile{}
}

func (erupe93BetaProfile) Name() string {
	return "9.3b"
}

func (erupe93BetaProfile) ReadyPath() string {
	return "/launcher"
}

func (erupe93BetaProfile) PublicPort(cfg BaseConfig) (int, error) {
	if !cfg.API.Enabled {
		return 0, fmt.Errorf("config.json has API.Enabled set to false")
	}
	if cfg.API.Port <= 0 {
		return 0, fmt.Errorf("config.json has invalid API.Port")
	}
	return cfg.API.Port, nil
}

func (erupe93BetaProfile) ResolveClientMode(cfg BaseConfig, fallback string) (string, error) {
	clientMode := strings.TrimSpace(cfg.ClientMode)
	if clientMode == "" {
		return "", fmt.Errorf("config.json has blank ClientMode")
	}
	return clientMode, nil
}

func (erupe93BetaProfile) RewriteConfig(cfg map[string]any, upstreamPort int) error {
	apiValue, ok := cfg["API"].(map[string]any)
	if !ok {
		return fmt.Errorf("config.json is missing API object")
	}
	apiValue["Enabled"] = true
	apiValue["Port"] = upstreamPort
	cfg["API"] = apiValue
	return nil
}
