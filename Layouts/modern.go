package layouts

import (
	"encoding/json"
	"fmt"
	"strings"
)

type DatabaseConfig struct {
	Host     string `json:"Host"`
	Port     int    `json:"Port"`
	User     string `json:"User"`
	Password string `json:"Password"`
	Database string `json:"Database"`
}

type APIConfig struct {
	Enabled            bool   `json:"Enabled"`
	Port               int    `json:"Port"`
	PatchServer        string `json:"PatchServer"`
	AltClient          bool   `json:"AltClient"`
	AltClientSaveCache bool   `json:"AltClientSaveCache"`
	AltClientLogs      bool   `json:"AltClientLogs"`
}

type SignV2Config struct {
	Enabled bool `json:"Enabled"`
	Port    int  `json:"Port"`
}

type DevModeOptionsConfig struct {
	DivaEvent   int  `json:"DivaEvent"`
	FestaEvent  int  `json:"FestaEvent"`
	MezFesEvent bool `json:"MezFesEvent"`
	MezFesAlt   bool `json:"MezFesAlt"`
}

type DebugOptionsConfig struct {
	DivaOverride       *int `json:"DivaOverride"`
	FestaOverride      *int `json:"FestaOverride"`
	TournamentOverride *int `json:"TournamentOverride"`
}

type GameplayOptionsConfig struct {
	FeaturedWeapons      int    `json:"FeaturedWeapons"`
	MezFesSoloTickets    uint32 `json:"MezFesSoloTickets"`
	MezFesGroupTickets   uint32 `json:"MezFesGroupTickets"`
	MezFesDuration       int    `json:"MezFesDuration"`
	MezFesSwitchMinigame bool   `json:"MezFesSwitchMinigame"`
}

type BaseConfig struct {
	ClientMode      string                `json:"ClientMode"`
	HideLoginNotice bool                  `json:"HideLoginNotice"`
	LoginNotices    []string              `json:"LoginNotices"`
	EarthStatus     int32                 `json:"EarthStatus"`
	EarthID         int32                 `json:"EarthID"`
	EarthMonsters   []int32               `json:"EarthMonsters"`
	DevModeOptions  DevModeOptionsConfig  `json:"DevModeOptions"`
	DebugOptions    DebugOptionsConfig    `json:"DebugOptions"`
	GameplayOptions GameplayOptionsConfig `json:"GameplayOptions"`
	Database        DatabaseConfig        `json:"Database"`
	API             APIConfig             `json:"API"`
	SignV2          SignV2Config          `json:"SignV2"`
}

type Profile interface {
	Name() string
	ReadyPath() string
	PublicPort(BaseConfig) (int, error)
	ResolveClientMode(BaseConfig, string) (string, error)
	RewriteConfig(map[string]any, int) error
}

type modernProfile struct{}

func ParseBaseConfig(raw []byte) (BaseConfig, error) {
	var cfg BaseConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func ResolveProfile(version string) (Profile, error) {
	switch strings.TrimSpace(version) {
	case "", "9.3+":
		return Modern(), nil
	case "9.2.1":
		return Erupe921(), nil
	case "9.2":
		return Erupe92(), nil
	default:
		return nil, fmt.Errorf("unsupported erupe_version %q", version)
	}
}

func Modern() Profile {
	return modernProfile{}
}

func (modernProfile) Name() string {
	return "9.3+"
}

func (modernProfile) ReadyPath() string {
	return "/health"
}

func (modernProfile) PublicPort(cfg BaseConfig) (int, error) {
	if !cfg.API.Enabled {
		return 0, fmt.Errorf("config.json has API.Enabled set to false")
	}
	if cfg.API.Port <= 0 {
		return 0, fmt.Errorf("config.json has invalid API.Port")
	}
	return cfg.API.Port, nil
}

func (modernProfile) ResolveClientMode(cfg BaseConfig, fallback string) (string, error) {
	clientMode := strings.TrimSpace(cfg.ClientMode)
	if clientMode == "" {
		return "", fmt.Errorf("config.json has blank ClientMode")
	}
	return clientMode, nil
}

func (modernProfile) RewriteConfig(cfg map[string]any, upstreamPort int) error {
	apiValue, ok := cfg["API"].(map[string]any)
	if !ok {
		return fmt.Errorf("config.json is missing API object")
	}
	apiValue["Enabled"] = true
	apiValue["Port"] = upstreamPort
	apiValue["AltClient"] = false
	apiValue["AltClientSaveCache"] = false
	apiValue["AltClientLogs"] = false
	cfg["API"] = apiValue
	return nil
}
