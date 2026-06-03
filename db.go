package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type altLauncherFriendRow struct {
	ID   uint32 `db:"id"`
	Name string `db:"name"`
}

type altClientCharacterRow struct {
	ID         uint32 `db:"id"`
	TimePlayed uint32 `db:"time_played"`
}

type altClientDistributionRow struct {
	ID              uint32 `db:"id"`
	EventName       string `db:"event_name"`
	Description     string `db:"description"`
	DistType        int32  `db:"type"`
	Deadline        int64  `db:"deadline"`
	TimesAcceptable uint32 `db:"times_acceptable"`
	MinHR           *int32 `db:"min_hr"`
	MaxHR           *int32 `db:"max_hr"`
	MinSR           *int32 `db:"min_sr"`
	MaxSR           *int32 `db:"max_sr"`
	MinGR           *int32 `db:"min_gr"`
	MaxGR           *int32 `db:"max_gr"`
	TotalCount      uint32 `db:"total_count"`
}

type altClientDistributionItemRow struct {
	DistributionID uint32 `db:"distribution_id"`
	ID             uint32 `db:"id"`
	ItemType       uint8  `db:"item_type"`
	ItemID         uint32 `db:"item_id"`
	Quantity       uint32 `db:"quantity"`
}

type altClientMailRow struct {
	SenderID          uint32 `db:"sender_id"`
	SenderName        string `db:"sender_name"`
	Subject           string `db:"subject"`
	Body              string `db:"body"`
	AttachedItem      uint32 `db:"attached_item"`
	AttachedItemCount uint32 `db:"attached_item_amount"`
	IsGuildInvite     bool   `db:"is_guild_invite"`
	CreatedAt         int64  `db:"created_at"`
	IsSystemMessage   bool   `db:"is_sys_message"`
}

type altClientOnlineFriendSessionRow struct {
	CharID   uint32 `db:"char_id"`
	ServerID uint32 `db:"server_id"`
}

type legacyAuthRow struct {
	UserID        uint32       `db:"user_id"`
	Rights        uint32       `db:"rights"`
	ReturnExpires sql.NullTime `db:"return_expires"`
}

type dbCapabilities struct {
	hasCharacterDeleted           bool
	hasCharacterIsNewCharacter    bool
	hasCharacterTimePlayed        bool
	hasCharacterSavedata          bool
	hasDistributionTable          bool
	hasDistributionItemsTable     bool
	hasDistributionsAcceptedTable bool
	hasEventsTable                bool
	hasFeatureWeaponTable         bool
}

type legacyEventRow struct {
	EventType  string `db:"event_type"`
	StartEpoch int64  `db:"start_epoch"`
}

type legacyFeatureWeaponRow struct {
	StartEpoch int64  `db:"start_epoch"`
	Featured   uint32 `db:"featured"`
}

var legacyCourseNames = map[uint16]string{
	1:  "Trial",
	2:  "HunterLife",
	3:  "Extra",
	4:  "ExtraB",
	5:  "Mobile",
	6:  "Premium",
	7:  "Pallone",
	8:  "Legend",
	9:  "N",
	10: "Secret",
	11: "Royal",
	12: "NBoost",
	13: "(unknown)",
	14: "(unknown)",
	15: "(unknown)",
	16: "(unknown)",
	17: "(unknown)",
	18: "(unknown)",
	19: "(unknown)",
	20: "DEBUG",
	21: "COG_LINK_EXPIRED",
	22: "360_GOLD",
	23: "PS3_TROP",
	24: "COG",
	25: "CAFE_SP",
	26: "Official",
	27: "CardHL",
	28: "CardEX",
	29: "Free",
	30: "NetCafe",
}

func openDatabase(cfg baseConfig) (*sqlx.DB, error) {
	connectString := fmt.Sprintf(
		"host='%s' port='%d' user='%s' password='%s' dbname='%s' sslmode=disable",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Database,
	)
	return sqlx.Connect("postgres", connectString)
}

func detectHasBansTable(ctx context.Context, db *sqlx.DB) (bool, error) {
	var tableName sql.NullString
	err := db.QueryRowContext(ctx, "SELECT to_regclass('public.bans')::text").Scan(&tableName)
	if err != nil {
		return false, err
	}
	return tableName.Valid && strings.TrimSpace(tableName.String) != "", nil
}

func detectTableExists(ctx context.Context, db *sqlx.DB, table string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, table).Scan(&exists)
	return exists, err
}

func detectColumnExists(ctx context.Context, db *sqlx.DB, table string, column string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		)
	`, table, column).Scan(&exists)
	return exists, err
}

func detectDBCapabilities(ctx context.Context, db *sqlx.DB) (dbCapabilities, error) {
	var caps dbCapabilities
	var err error

	if caps.hasCharacterDeleted, err = detectColumnExists(ctx, db, "characters", "deleted"); err != nil {
		return caps, err
	}
	if caps.hasCharacterIsNewCharacter, err = detectColumnExists(ctx, db, "characters", "is_new_character"); err != nil {
		return caps, err
	}
	if caps.hasCharacterTimePlayed, err = detectColumnExists(ctx, db, "characters", "time_played"); err != nil {
		return caps, err
	}
	if caps.hasCharacterSavedata, err = detectColumnExists(ctx, db, "characters", "savedata"); err != nil {
		return caps, err
	}
	if caps.hasDistributionTable, err = detectTableExists(ctx, db, "distribution"); err != nil {
		return caps, err
	}
	if caps.hasDistributionItemsTable, err = detectTableExists(ctx, db, "distribution_items"); err != nil {
		return caps, err
	}
	if caps.hasDistributionsAcceptedTable, err = detectTableExists(ctx, db, "distributions_accepted"); err != nil {
		return caps, err
	}
	if caps.hasEventsTable, err = detectTableExists(ctx, db, "events"); err != nil {
		return caps, err
	}
	if caps.hasFeatureWeaponTable, err = detectTableExists(ctx, db, "feature_weapon"); err != nil {
		return caps, err
	}

	return caps, nil
}

func (a *app) characterVisibilityFilter(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	parts := make([]string, 0, 2)
	if a.dbCapabilities.hasCharacterDeleted {
		parts = append(parts, prefix+"deleted = false")
	}
	if a.dbCapabilities.hasCharacterIsNewCharacter {
		parts = append(parts, prefix+"is_new_character = false")
	}
	if len(parts) == 0 {
		return ""
	}
	return " AND " + strings.Join(parts, " AND ")
}

func (a *app) lookupUserIDByUsername(ctx context.Context, username string) (uint32, error) {
	var userID uint32
	err := a.db.QueryRowContext(ctx, "SELECT id FROM users WHERE username = $1", username).Scan(&userID)
	return userID, err
}

func (a *app) userIDFromToken(ctx context.Context, token string) (uint32, error) {
	var userID uint32
	err := a.db.QueryRowContext(ctx, "SELECT user_id FROM sign_sessions WHERE token = $1", token).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("invalid login token")
	}
	return userID, err
}

func (a *app) legacyAuthDataForToken(ctx context.Context, token string) (legacyAuthRow, error) {
	var row legacyAuthRow
	err := a.db.GetContext(ctx, &row, `
		SELECT
			s.user_id,
			COALESCE(u.rights, 0) AS rights,
			u.return_expires
		FROM sign_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = $1
		LIMIT 1
	`, token)
	return row, err
}

func (a *app) altClientBanStatus(ctx context.Context, userID uint32) (bool, *time.Time, error) {
	if !a.hasBansTable {
		return false, nil, nil
	}

	var expires sql.NullTime
	err := a.db.QueryRowContext(ctx, "SELECT expires FROM bans WHERE user_id = $1 LIMIT 1", userID).Scan(&expires)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if !expires.Valid {
		return true, nil, nil
	}
	if expires.Time.After(time.Now()) {
		banExpiry := expires.Time.UTC()
		return true, &banExpiry, nil
	}
	return false, nil, nil
}

func formatAltClientBanMessage(expires *time.Time) string {
	if expires == nil {
		return "This account is permanently banned."
	}
	return fmt.Sprintf("This account is banned until %s UTC.", expires.UTC().Format("2006-01-02 15:04:05"))
}

func parseAltLauncherFriendIDs(csv string) []int {
	parts := strings.Split(csv, ",")
	ids := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, raw := range parts {
		clean := strings.TrimSpace(raw)
		if clean == "" {
			continue
		}
		id, err := strconv.Atoi(clean)
		if err != nil || id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func markReturning(characters []authCharacter) {
	ninetyDaysAgo := time.Now().Add(-90 * 24 * time.Hour)
	for i := range characters {
		characters[i].Returning = time.Unix(int64(characters[i].LastLogin), 0).Before(ninetyDaysAgo)
	}
}

func (a *app) legacyCharactersForUser(ctx context.Context, userID uint32) ([]authCharacter, error) {
	characters := make([]authCharacter, 0)
	err := a.db.SelectContext(ctx, &characters, `
		SELECT id, name, is_female, weapon_type, hrp AS hr, gr, last_login
		FROM characters
		WHERE user_id = $1 AND deleted = false AND is_new_character = false
		ORDER BY id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	markReturning(characters)
	return characters, nil
}

func (a *app) legacyCharacterByID(ctx context.Context, userID uint32, charID uint32) (authCharacter, error) {
	var character authCharacter
	err := a.db.GetContext(ctx, &character, `
		SELECT id, name, is_female, weapon_type, hrp AS hr, gr, last_login
		FROM characters
		WHERE user_id = $1 AND id = $2
		LIMIT 1
	`, userID, charID)
	if err != nil {
		return character, err
	}
	ninetyDaysAgo := time.Now().Add(-90 * 24 * time.Hour)
	character.Returning = time.Unix(int64(character.LastLogin), 0).Before(ninetyDaysAgo)
	return character, nil
}

func (a *app) exportCharacterData(ctx context.Context, userID uint32, charID uint32) (map[string]any, error) {
	var character map[string]any
	err := a.db.QueryRowContext(ctx, `
		SELECT row_to_json(c)
		FROM (
			SELECT *
			FROM characters
			WHERE user_id = $1 AND id = $2
			LIMIT 1
		) c
	`, userID, charID).Scan(&character)
	return character, err
}

func (a *app) altLauncherFriendsForCharacters(ctx context.Context, chars []authCharacter) ([]altLauncherFriend, error) {
	friends := make([]altLauncherFriend, 0)
	if len(chars) == 0 {
		return friends, nil
	}

	for _, char := range chars {
		var friendsCSV string
		if err := a.db.QueryRowContext(ctx, "SELECT friends FROM characters WHERE id=$1", char.ID).Scan(&friendsCSV); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}

		ids := parseAltLauncherFriendIDs(friendsCSV)
		if len(ids) == 0 {
			continue
		}

		rows := make([]altLauncherFriendRow, 0)
		if err := a.db.SelectContext(ctx, &rows, "SELECT id, name FROM characters WHERE id = ANY($1)", pq.Array(ids)); err != nil {
			return nil, err
		}

		for _, row := range rows {
			friends = append(friends, altLauncherFriend{CID: char.ID, ID: row.ID, Name: row.Name})
		}
	}

	return friends, nil
}

func altClientDistributionTypeLabel(distType int32) string {
	switch distType {
	case 0:
		return "Bought"
	case 1:
		return "Event"
	case 2:
		return "Compensation"
	case 4:
		return "Promotion"
	case 6:
		return "Subscription"
	case 7:
		return "Event Item"
	case 8:
		return "Promotion Item"
	case 9:
		return "Subscription Item"
	default:
		return "NO_LABEL"
	}
}

func altClientMailLabel(isGuildInvite bool, isSystemMessage bool) string {
	if isGuildInvite {
		return "Guild Invite"
	}
	if isSystemMessage {
		return "System"
	}
	return ""
}

func (a *app) altClientUserStats(ctx context.Context, userID uint32) (altClientUserStats, error) {
	var out altClientUserStats
	err := a.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(gacha_premium, 0),
			COALESCE(gacha_trial, 0),
			COALESCE(frontier_points, 0)
		FROM users
		WHERE id = $1
	`, userID).Scan(&out.GachaPremium, &out.GachaTrial, &out.FrontierPoints)
	return out, err
}

func (a *app) altClientCharacterRows(ctx context.Context, userID uint32) ([]altClientCharacterRow, error) {
	rows := make([]altClientCharacterRow, 0)
	timePlayedExpr := "0 AS time_played"
	if a.dbCapabilities.hasCharacterTimePlayed {
		timePlayedExpr = "COALESCE(time_played, 0) AS time_played"
	}
	query := fmt.Sprintf(`
		SELECT
			id,
			%s
		FROM characters
		WHERE user_id = $1%s
		ORDER BY id ASC
	`, timePlayedExpr, a.characterVisibilityFilter(""))
	err := a.db.SelectContext(ctx, &rows, query, userID)
	return rows, err
}

func (a *app) altClientOnlineFriends(ctx context.Context, charRows []altClientCharacterRow) ([]altClientOnlineFriend, error) {
	out := make([]altClientOnlineFriend, 0)
	if len(charRows) == 0 {
		return out, nil
	}

	chars := make([]authCharacter, 0, len(charRows))
	for _, row := range charRows {
		chars = append(chars, authCharacter{ID: row.ID})
	}

	friends, err := a.altLauncherFriendsForCharacters(ctx, chars)
	if err != nil || len(friends) == 0 {
		return out, err
	}

	friendIDs := make([]int, 0, len(friends))
	seenFriendIDs := make(map[int]struct{}, len(friends))
	for _, friend := range friends {
		friendID := int(friend.ID)
		if friendID <= 0 {
			continue
		}
		if _, seen := seenFriendIDs[friendID]; seen {
			continue
		}
		seenFriendIDs[friendID] = struct{}{}
		friendIDs = append(friendIDs, friendID)
	}
	if len(friendIDs) == 0 {
		return out, nil
	}

	sessionRows := make([]altClientOnlineFriendSessionRow, 0)
	err = a.db.SelectContext(ctx, &sessionRows, `
		SELECT
			char_id,
			MAX(server_id) AS server_id
		FROM sign_sessions
		WHERE char_id = ANY($1)
			AND char_id IS NOT NULL
			AND server_id IS NOT NULL
		GROUP BY char_id
	`, pq.Array(friendIDs))
	if err != nil {
		return nil, err
	}
	if len(sessionRows) == 0 {
		return out, nil
	}

	serverIDByFriendID := make(map[uint32]uint32, len(sessionRows))
	for _, row := range sessionRows {
		serverIDByFriendID[row.CharID] = row.ServerID
	}

	for _, friend := range friends {
		serverID, online := serverIDByFriendID[friend.ID]
		if !online {
			continue
		}
		out = append(out, altClientOnlineFriend{
			CID:      friend.CID,
			ID:       friend.ID,
			Name:     friend.Name,
			ServerID: serverID,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CID != out[j].CID {
			return out[i].CID < out[j].CID
		}
		nameI := strings.ToLower(strings.TrimSpace(out[i].Name))
		nameJ := strings.ToLower(strings.TrimSpace(out[j].Name))
		if nameI != nameJ {
			return nameI < nameJ
		}
		return out[i].ID < out[j].ID
	})

	return out, nil
}

func (a *app) altClientCharacterSavedata(ctx context.Context, userID uint32, charID uint32) ([]byte, error) {
	if !a.dbCapabilities.hasCharacterSavedata {
		return nil, sql.ErrNoRows
	}
	var savedata []byte
	query := fmt.Sprintf(`
		SELECT savedata
		FROM characters
		WHERE id = $1
			AND user_id = $2%s
		LIMIT 1
	`, a.characterVisibilityFilter(""))
	err := a.db.QueryRowContext(ctx, query, charID, userID).Scan(&savedata)
	return savedata, err
}

func (a *app) altClientUnclaimedDistributions(ctx context.Context, charID uint32, limit uint32, offset uint32) ([]altClientDistribution, uint32, error) {
	if !a.dbCapabilities.hasDistributionTable || !a.dbCapabilities.hasDistributionsAcceptedTable {
		return []altClientDistribution{}, 0, nil
	}

	if limit == 0 {
		limit = 6
	}
	if limit > 24 {
		limit = 24
	}

	rows := make([]altClientDistributionRow, 0)
	hrColumn := "c.hr"
	if a.isLegacyLayout() {
		hrColumn = "c.hrp"
	}
	err := a.db.SelectContext(ctx, &rows, `
		WITH char_rank AS (
			SELECT
				COALESCE(`+hrColumn+`, 0) AS hr_rank,
				COALESCE(c.gr, 0) AS gr_rank,
				CASE
					WHEN COALESCE(NULLIF(BTRIM(to_jsonb(c)->>'sr'), ''), '') ~ '^-?[0-9]+$'
						THEN (to_jsonb(c)->>'sr')::int
					ELSE 0
				END AS sr_rank
			FROM characters c
			WHERE c.id = $1
		)
		SELECT
			d.id,
			COALESCE(NULLIF(BTRIM(d.event_name), ''), 'Distribution #' || d.id::text) AS event_name,
			COALESCE(d.description, '') AS description,
			COALESCE(d.type, 3) AS type,
			COALESCE(EXTRACT(EPOCH FROM d.deadline)::bigint, 0) AS deadline,
			COALESCE(d.times_acceptable, 1) AS times_acceptable,
			d.min_hr AS min_hr,
			d.max_hr AS max_hr,
			d.min_sr AS min_sr,
			d.max_sr AS max_sr,
			d.min_gr AS min_gr,
			d.max_gr AS max_gr,
			COUNT(*) OVER() AS total_count
		FROM distribution d
		JOIN char_rank cr ON true
		WHERE
			(d.character_id = $1 OR d.character_id IS NULL)
			AND (d.deadline IS NULL OR d.deadline > now())
			AND (COALESCE(d.min_hr, -1) < 0 OR cr.hr_rank >= d.min_hr)
			AND (COALESCE(d.max_hr, -1) < 0 OR cr.hr_rank <= d.max_hr)
			AND (COALESCE(d.min_sr, -1) < 0 OR cr.sr_rank >= d.min_sr)
			AND (COALESCE(d.max_sr, -1) < 0 OR cr.sr_rank <= d.max_sr)
			AND (COALESCE(d.min_gr, -1) < 0 OR cr.gr_rank >= d.min_gr)
			AND (COALESCE(d.max_gr, -1) < 0 OR cr.gr_rank <= d.max_gr)
			AND COALESCE(d.times_acceptable, 1) > (
				SELECT COUNT(*)
				FROM distributions_accepted da
				WHERE da.distribution_id = d.id AND da.character_id = $1
			)
		ORDER BY d.id ASC
		LIMIT $2 OFFSET $3
	`, charID, int(limit), int(offset))
	if err != nil {
		return nil, 0, err
	}

	totalCount := uint32(0)
	if len(rows) > 0 {
		totalCount = rows[0].TotalCount
	}

	itemsByDistributionID := map[uint32][]altClientDistributionItem{}
	if a.dbCapabilities.hasDistributionItemsTable && len(rows) > 0 {
		distIDs := make([]uint32, 0, len(rows))
		for _, row := range rows {
			distIDs = append(distIDs, row.ID)
		}

		itemRows := make([]altClientDistributionItemRow, 0)
		if err := a.db.SelectContext(ctx, &itemRows, `
			SELECT
				distribution_id,
				COALESCE(id, 0) AS id,
				COALESCE(item_type, 0) AS item_type,
				COALESCE(item_id, 0) AS item_id,
				COALESCE(quantity, 0) AS quantity
			FROM distribution_items
			WHERE distribution_id = ANY($1)
			ORDER BY distribution_id, id
		`, pq.Array(distIDs)); err != nil {
			return nil, 0, err
		}

		for _, itemRow := range itemRows {
			itemsByDistributionID[itemRow.DistributionID] = append(
				itemsByDistributionID[itemRow.DistributionID],
				altClientDistributionItem{
					ID:       itemRow.ID,
					ItemType: itemRow.ItemType,
					ItemID:   itemRow.ItemID,
					Quantity: itemRow.Quantity,
				},
			)
		}
	}

	out := make([]altClientDistribution, 0, len(rows))
	for _, row := range rows {
		out = append(out, altClientDistribution{
			ID:              row.ID,
			EventName:       row.EventName,
			Description:     row.Description,
			Type:            row.DistType,
			TypeLabel:       altClientDistributionTypeLabel(row.DistType),
			Deadline:        row.Deadline,
			TimesAcceptable: row.TimesAcceptable,
			MinHR:           row.MinHR,
			MaxHR:           row.MaxHR,
			MinSR:           row.MinSR,
			MaxSR:           row.MaxSR,
			MinGR:           row.MinGR,
			MaxGR:           row.MaxGR,
			Items:           itemsByDistributionID[row.ID],
		})
	}
	return out, totalCount, nil
}

func normalizeLegacySpecialEventLabel(eventType string) string {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "vs":
		return "VS"
	case "mezfes":
		return "MezFes"
	default:
		clean := strings.TrimSpace(eventType)
		if clean == "" {
			return ""
		}
		return strings.ToUpper(clean)
	}
}

func appendUniqueString(values []string, next string) []string {
	clean := strings.TrimSpace(next)
	if clean == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), clean) {
			return values
		}
	}
	return append(values, clean)
}

func unixSeconds(t time.Time) uint32 {
	if t.IsZero() {
		return 0
	}
	ts := t.UTC().Unix()
	if ts <= 0 {
		return 0
	}
	return uint32(ts)
}

func unixSecondsFromEpoch(ts int64) uint32 {
	if ts <= 0 {
		return 0
	}
	return uint32(ts)
}

func legacyAdjustedTime() time.Time {
	baseTime := time.Now().In(time.FixedZone("UTC+9", 9*60*60))
	return time.Date(baseTime.Year(), baseTime.Month(), baseTime.Day(), baseTime.Hour(), baseTime.Minute(), baseTime.Second(), baseTime.Nanosecond(), baseTime.Location())
}

func legacyMidnight() time.Time {
	baseTime := time.Now().In(time.FixedZone("UTC+9", 9*60*60))
	return time.Date(baseTime.Year(), baseTime.Month(), baseTime.Day(), 0, 0, 0, 0, baseTime.Location())
}

func legacyWeekStart() time.Time {
	midnight := legacyMidnight()
	offset := (int(midnight.Weekday()) - 1) * -24
	return midnight.Add(time.Hour * time.Duration(offset))
}

func legacyWeekNext() time.Time {
	return legacyWeekStart().Add(time.Hour * 24 * 7)
}

func legacyCourseListFromRights(rights uint32) []authCourse {
	courses := []authCourse{{ID: 1, Name: legacyCourseNames[1]}}
	normalCafeCourseSet := false
	netCafeCourseSet := false

	for id := 31; id >= 0; id-- {
		value := uint32(1) << uint(id)
		if rights >= value {
			switch uint16(id) {
			case 26:
				if !normalCafeCourseSet {
					normalCafeCourseSet = true
					courses = append(courses, authCourse{ID: 25, Name: legacyCourseNames[25]})
				}
				fallthrough
			case 9:
				if !netCafeCourseSet {
					netCafeCourseSet = true
					courses = append(courses, authCourse{ID: 30, Name: legacyCourseNames[30]})
				}
			}

			courses = append(courses, authCourse{
				ID:   uint16(id),
				Name: legacyCourseNames[uint16(id)],
			})
			rights -= value
		}

		if id == 0 {
			break
		}
	}

	return courses
}

func (a *app) legacyFestaOverride() int {
	if a.baseConfig.DebugOptions.FestaOverride != nil {
		return *a.baseConfig.DebugOptions.FestaOverride
	}
	if a.baseConfig.DevModeOptions.FestaEvent != 0 {
		return a.baseConfig.DevModeOptions.FestaEvent
	}
	return -1
}

func (a *app) legacyDivaOverride() int {
	if a.baseConfig.DebugOptions.DivaOverride != nil {
		return *a.baseConfig.DebugOptions.DivaOverride
	}
	return a.baseConfig.DevModeOptions.DivaEvent
}

func (a *app) legacyTournamentOverride() int {
	if a.baseConfig.DebugOptions.TournamentOverride != nil {
		return *a.baseConfig.DebugOptions.TournamentOverride
	}
	return 0
}

func (a *app) legacyMezFesTickets() (uint32, uint32) {
	soloTickets := a.baseConfig.GameplayOptions.MezFesSoloTickets
	groupTickets := a.baseConfig.GameplayOptions.MezFesGroupTickets
	if soloTickets == 0 {
		soloTickets = 5
	}
	if groupTickets == 0 {
		groupTickets = 1
	}
	return soloTickets, groupTickets
}

func (a *app) legacyMezFesStalls() []uint32 {
	stalls := []uint32{10, 3, 6, 9, 4, 8, 5, 7}
	if a.baseConfig.DevModeOptions.MezFesAlt || a.baseConfig.GameplayOptions.MezFesSwitchMinigame {
		stalls[4] = 2
	}
	return stalls
}

func (a *app) legacyMezFesFromWindow(start time.Time, end time.Time) *altClientMezFes {
	soloTickets, groupTickets := a.legacyMezFesTickets()

	return &altClientMezFes{
		ID:           unixSeconds(start),
		Start:        unixSeconds(start),
		End:          unixSeconds(end),
		SoloTickets:  soloTickets,
		GroupTickets: groupTickets,
		Stalls:       a.legacyMezFesStalls(),
	}
}

func (a *app) legacyMezFes() *altClientMezFes {
	if !a.baseConfig.DevModeOptions.MezFesEvent {
		return nil
	}
	duration := a.baseConfig.GameplayOptions.MezFesDuration
	if duration <= 0 {
		duration = 172800
	}
	return a.legacyMezFesFromWindow(legacyWeekStart().Add(-time.Duration(duration)*time.Second), legacyWeekNext())
}

func (a *app) legacyMezFesFromEvent(startEpoch int64) *altClientMezFes {
	if startEpoch <= 0 {
		return nil
	}
	duration := a.baseConfig.GameplayOptions.MezFesDuration
	if duration <= 0 {
		duration = 172800
	}
	start := time.Unix(startEpoch, 0)
	return a.legacyMezFesFromWindow(start, start.Add(time.Duration(duration)*time.Second))
}

func legacyDBTimedEventActive(startEpoch int64) bool {
	if startEpoch <= 0 {
		return false
	}

	now := legacyAdjustedTime().Unix()
	return now >= startEpoch && now <= startEpoch+2977200
}

func legacyEarthStatusEvents(status int32) []string {
	switch status {
	case 1, 2:
		return []string{"Conquest"}
	case 11, 12:
		return []string{"Pallone Festival"}
	case 21:
		return []string{"Tower"}
	default:
		return nil
	}
}

func (a *app) legacyServerStatus(ctx context.Context) (serverStatusResponse, error) {
	out := serverStatusResponse{
		Events: altClientEvents{},
	}

	festaMode := a.legacyFestaOverride()
	switch {
	case festaMode > 0:
		out.Events.FestaActive = true
	case festaMode == 0:
		out.Events.FestaActive = false
	}

	divaMode := a.legacyDivaOverride()
	switch {
	case divaMode > 0:
		out.Events.DivaActive = true
	case divaMode == 0:
		out.Events.DivaActive = false
	}

	tournamentMode := a.legacyTournamentOverride()
	switch {
	case tournamentMode > 0:
		out.Events.TournamentActive = true
	case tournamentMode == 0:
		out.Events.TournamentActive = false
	}

	out.MezFes = a.legacyMezFes()

	for _, eventName := range legacyEarthStatusEvents(a.baseConfig.EarthStatus) {
		out.Events.SpecialEvents = appendUniqueString(out.Events.SpecialEvents, eventName)
	}

	if a.dbCapabilities.hasEventsTable {
		rows := make([]legacyEventRow, 0)
		if err := a.db.SelectContext(ctx, &rows, `
			SELECT DISTINCT
				event_type,
				COALESCE(EXTRACT(epoch FROM start_time)::bigint, 0) AS start_epoch
			FROM events
			WHERE start_time <= now()
		`); err != nil {
			return out, err
		}
		for _, row := range rows {
			switch strings.ToLower(strings.TrimSpace(row.EventType)) {
			case "festa":
				if festaMode < 0 {
					out.Events.FestaActive = legacyDBTimedEventActive(row.StartEpoch)
				}
			case "diva":
				if divaMode < 0 {
					out.Events.DivaActive = legacyDBTimedEventActive(row.StartEpoch)
				}
			case "vs":
				// The server schema allows "vs", but the channel handler only
				// enables the tournament from DebugOptions.TournamentOverride.
			case "mezfes":
				mezFes := a.legacyMezFesFromEvent(row.StartEpoch)
				if mezFes != nil && mezFes.Start <= unixSeconds(legacyAdjustedTime()) && unixSeconds(legacyAdjustedTime()) <= mezFes.End {
					if out.MezFes == nil {
						out.MezFes = mezFes
					}
					out.Events.SpecialEvents = appendUniqueString(out.Events.SpecialEvents, normalizeLegacySpecialEventLabel(row.EventType))
				}
			default:
				if legacyDBTimedEventActive(row.StartEpoch) {
					out.Events.SpecialEvents = appendUniqueString(out.Events.SpecialEvents, normalizeLegacySpecialEventLabel(row.EventType))
				}
			}
		}
	}

	if a.dbCapabilities.hasFeatureWeaponTable {
		var row legacyFeatureWeaponRow
		err := a.db.GetContext(ctx, &row, `
			SELECT
				COALESCE(EXTRACT(epoch FROM start_time)::bigint, 0) AS start_epoch,
				featured
			FROM feature_weapon
			WHERE start_time <= now()
			ORDER BY start_time DESC
			LIMIT 1
		`)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		if err == nil {
			out.FeaturedWeapon = &altClientFeaturedWeapon{
				StartTime:      unixSecondsFromEpoch(row.StartEpoch),
				ActiveFeatures: row.Featured,
			}
		}
	}

	return out, nil
}

func (a *app) legacyDashboardStats(ctx context.Context) (dashboardStatsResponse, error) {
	var out dashboardStatsResponse
	err := a.db.QueryRowContext(ctx, `
		SELECT COALESCE(COUNT(DISTINCT char_id), 0)
		FROM sign_sessions
		WHERE char_id IS NOT NULL
			AND char_id > 0
			AND server_id IS NOT NULL
	`).Scan(&out.OnlinePlayers)
	return out, err
}

func (a *app) altClientUnreadMails(ctx context.Context, charID uint32) ([]altClientMail, error) {
	rows := make([]altClientMailRow, 0)
	query := `
		SELECT
			COALESCE(m.sender_id, 0) AS sender_id,
			COALESCE(sc.name, CASE WHEN COALESCE(m.sender_id, 0) = 0 THEN 'System' ELSE 'Unknown' END) AS sender_name,
			COALESCE(m.subject, '') AS subject,
			COALESCE(m.body, '') AS body,
			COALESCE(m.attached_item, 0) AS attached_item,
			COALESCE(m.attached_item_amount, 0) AS attached_item_amount,
			COALESCE(m.is_guild_invite, false) AS is_guild_invite,
			COALESCE(EXTRACT(EPOCH FROM m.created_at)::bigint, 0) AS created_at,
			COALESCE(m.is_sys_message, false) AS is_sys_message
		FROM mail m
		LEFT JOIN characters sc ON sc.id = m.sender_id
		WHERE m.recipient_id = $1 AND m.deleted = false AND m.read = false
		ORDER BY m.created_at DESC
	`
	if a.isLegacyLayout() {
		query = `
			SELECT
				COALESCE(m.sender_id, 0) AS sender_id,
				COALESCE(sc.name, CASE WHEN COALESCE(m.sender_id, 0) = 0 THEN 'System' ELSE 'Unknown' END) AS sender_name,
				COALESCE(m.subject, '') AS subject,
				COALESCE(m.body, '') AS body,
				COALESCE(m.attached_item, 0) AS attached_item,
				COALESCE(m.attached_item_amount, 0) AS attached_item_amount,
				COALESCE(m.is_guild_invite, false) AS is_guild_invite,
				COALESCE(EXTRACT(EPOCH FROM m.created_at)::bigint, 0) AS created_at,
				false AS is_sys_message
			FROM mail m
			LEFT JOIN characters sc ON sc.id = m.sender_id
			WHERE m.recipient_id = $1 AND m.deleted = false AND m.read = false
			ORDER BY m.created_at DESC
		`
	}
	if err := a.db.SelectContext(ctx, &rows, query, charID); err != nil {
		return nil, err
	}

	out := make([]altClientMail, 0, len(rows))
	for _, row := range rows {
		hasItem := row.AttachedItem > 0
		out = append(out, altClientMail{
			SenderID:           row.SenderID,
			SenderName:         row.SenderName,
			Subject:            row.Subject,
			Body:               row.Body,
			HasItem:            hasItem,
			AttachedItem:       row.AttachedItem,
			AttachedItemAmount: row.AttachedItemCount,
			ItemAmount:         row.AttachedItemCount,
			IsGuildInvite:      row.IsGuildInvite,
			CreatedAt:          row.CreatedAt,
			IsSystemMessage:    row.IsSystemMessage,
			Label:              altClientMailLabel(row.IsGuildInvite, row.IsSystemMessage),
		})
	}
	return out, nil
}
