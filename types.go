package main

type launcherBanner struct {
	Src  string `json:"src"`
	Link string `json:"link"`
}

type launcherMessage struct {
	Message string `json:"message"`
	Date    int64  `json:"date"`
	Link    string `json:"link"`
	Kind    int    `json:"kind"`
}

type launcherLink struct {
	Name string `json:"name"`
	Link string `json:"link"`
	Icon string `json:"icon"`
}

type launcherPS4Assets struct {
	Background      string              `json:"background,omitempty"`
	Button          string              `json:"button,omitempty"`
	AddServerButton string              `json:"addServerButton,omitempty"`
	Capcom          string              `json:"capcom,omitempty"`
	Cog             string              `json:"cog,omitempty"`
	Emblem          string              `json:"emblem,omitempty"`
	Headers         *launcherPS4Headers `json:"headers,omitempty"`
}

type launcherPS4Headers struct {
	Online  string `json:"online,omitempty"`
	Forward string `json:"forward,omitempty"`
	G       string `json:"g,omitempty"`
	Z       string `json:"z,omitempty"`
	ZZ      string `json:"zz,omitempty"`
}

type launcherHeaders struct {
	Online  string `json:"online,omitempty"`
	Forward string `json:"forward,omitempty"`
	G       string `json:"g,omitempty"`
	Z       string `json:"z,omitempty"`
	ZZ      string `json:"zz,omitempty"`
}

type launcherResponse struct {
	Banners                []launcherBanner   `json:"banners"`
	Messages               []launcherMessage  `json:"messages"`
	Links                  []launcherLink     `json:"links"`
	Tag                    string             `json:"tag,omitempty"`
	ServerTag              string             `json:"serverTag,omitempty"`
	Background             string             `json:"background,omitempty"`
	Cog                    string             `json:"cog,omitempty"`
	Capcom                 string             `json:"capcom,omitempty"`
	Button                 string             `json:"button,omitempty"`
	ClassicAddServerButton string             `json:"classicAddServerButton,omitempty"`
	Headers                *launcherHeaders   `json:"headers,omitempty"`
	Dialog                 string             `json:"dialog,omitempty"`
	ServerPatch            string             `json:"server_patch,omitempty"`
	PS4                    *launcherPS4Assets `json:"ps4,omitempty"`
}

type versionResponse struct {
	ClientMode string `json:"clientMode"`
	Name       string `json:"name"`
}

type authUser struct {
	TokenID uint32 `json:"tokenId"`
	Token   string `json:"token"`
	Rights  uint32 `json:"rights"`
}

type authCharacter struct {
	ID        uint32 `json:"id" db:"id"`
	Name      string `json:"name" db:"name"`
	IsFemale  bool   `json:"isFemale" db:"is_female"`
	Weapon    uint32 `json:"weapon" db:"weapon_type"`
	HR        uint32 `json:"hr" db:"hr"`
	GR        uint32 `json:"gr" db:"gr"`
	LastLogin int32  `json:"lastLogin" db:"last_login"`
	Returning bool   `json:"returning"`
}

type authCourse struct {
	ID   uint16 `json:"id"`
	Name string `json:"name"`
}

type authData struct {
	CurrentTS          uint32              `json:"currentTs"`
	ExpiryTS           uint32              `json:"expiryTs"`
	EntranceCount      uint32              `json:"entranceCount"`
	Notices            []string            `json:"notices"`
	User               authUser            `json:"user"`
	Characters         []authCharacter     `json:"characters"`
	Courses            []authCourse        `json:"courses"`
	MezFes             *altClientMezFes    `json:"mezFes"`
	Friends            []altLauncherFriend `json:"friends,omitempty"`
	PatchServer        string              `json:"patchServer"`
	AltSavedataEnabled bool                `json:"altSavedataEnabled,omitempty"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type altLauncherFriend struct {
	CID  uint32 `json:"cid"`
	ID   uint32 `json:"id"`
	Name string `json:"name"`
}

type altClientOnlineFriend struct {
	CID      uint32 `json:"cid"`
	ID       uint32 `json:"id"`
	Name     string `json:"name"`
	ServerID uint32 `json:"serverId"`
}

type altClientUserStats struct {
	GachaPremium   uint32 `json:"gachaPremium"`
	GachaTrial     uint32 `json:"gachaTrial"`
	FrontierPoints uint32 `json:"frontierPoints"`
}

type altClientMezFes struct {
	ID           uint32   `json:"id"`
	Start        uint32   `json:"start"`
	End          uint32   `json:"end"`
	SoloTickets  uint32   `json:"soloTickets"`
	GroupTickets uint32   `json:"groupTickets"`
	Stalls       []uint32 `json:"stalls,omitempty"`
}

type altClientFeaturedWeapon struct {
	StartTime      uint32 `json:"startTime"`
	ActiveFeatures uint32 `json:"activeFeatures"`
}

type altClientEvents struct {
	FestaActive      bool     `json:"festaActive"`
	DivaActive       bool     `json:"divaActive"`
	TournamentActive bool     `json:"tournamentActive,omitempty"`
	SpecialEvents    []string `json:"specialEvents,omitempty"`
}

type altClientDistribution struct {
	ID              uint32                      `json:"id"`
	EventName       string                      `json:"eventName"`
	Description     string                      `json:"description"`
	Type            int32                       `json:"type"`
	TypeLabel       string                      `json:"typeLabel"`
	Deadline        int64                       `json:"deadline,omitempty"`
	TimesAcceptable uint32                      `json:"timesAcceptable"`
	MinHR           *int32                      `json:"minHr,omitempty"`
	MaxHR           *int32                      `json:"maxHr,omitempty"`
	MinSR           *int32                      `json:"minSr,omitempty"`
	MaxSR           *int32                      `json:"maxSr,omitempty"`
	MinGR           *int32                      `json:"minGr,omitempty"`
	MaxGR           *int32                      `json:"maxGr,omitempty"`
	Items           []altClientDistributionItem `json:"items,omitempty"`
}

type altClientDistributionItem struct {
	ID       uint32 `json:"id"`
	ItemType uint8  `json:"itemType"`
	ItemID   uint32 `json:"itemId"`
	Quantity uint32 `json:"quantity"`
}

type altClientMail struct {
	SenderID           uint32 `json:"senderId"`
	SenderName         string `json:"senderName"`
	Subject            string `json:"subject"`
	Body               string `json:"body"`
	HasItem            bool   `json:"hasItem"`
	AttachedItem       uint32 `json:"attachedItem"`
	AttachedItemAmount uint32 `json:"attachedItemAmount"`
	ItemAmount         uint32 `json:"itemAmount"`
	IsGuildInvite      bool   `json:"isGuildInvite"`
	CreatedAt          int64  `json:"createdAt,omitempty"`
	IsSystemMessage    bool   `json:"isSystemMessage"`
	Label              string `json:"label,omitempty"`
}

type altClientCharacterStats struct {
	ID                           uint32                  `json:"id"`
	TimePlayed                   uint32                  `json:"timePlayed"`
	UnreadMail                   uint32                  `json:"unreadMail"`
	UnreadMailEntries            []altClientMail         `json:"unreadMailEntries,omitempty"`
	UnclaimedDistributions       uint32                  `json:"unclaimedDistributions"`
	UnclaimedDistributionNames   []string                `json:"unclaimedDistributionNames,omitempty"`
	UnclaimedDistributionDetails []altClientDistribution `json:"unclaimedDistributionDetails,omitempty"`
}

type altClientStatsResponse struct {
	User          altClientUserStats        `json:"user"`
	Characters    []altClientCharacterStats `json:"characters"`
	OnlineFriends []altClientOnlineFriend   `json:"onlineFriends"`
}

type altClientDistributionPageResponse struct {
	CharacterID uint32                  `json:"characterId"`
	Offset      uint32                  `json:"offset"`
	Limit       uint32                  `json:"limit"`
	Total       uint32                  `json:"total"`
	Entries     []altClientDistribution `json:"entries"`
}

type altClientSavedataResponse struct {
	CharacterID uint32 `json:"characterId"`
	Savedata    string `json:"savedata"`
	ClientMode  string `json:"clientMode,omitempty"`
}

type serverStatusResponse struct {
	MezFes         *altClientMezFes         `json:"mezFes,omitempty"`
	FeaturedWeapon *altClientFeaturedWeapon `json:"featuredWeapon,omitempty"`
	Events         altClientEvents          `json:"events"`
}

type dashboardStatsResponse struct {
	OnlinePlayers uint32 `json:"onlinePlayers"`
}
