package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type legacyLauncherMessage struct {
	Message string `json:"message"`
	Date    int64  `json:"date"`
	Link    string `json:"link"`
}

type legacyLauncherPayload struct {
	Important []legacyLauncherMessage `json:"important"`
	Normal    []legacyLauncherMessage `json:"normal"`
}

type legacyLoginPayload struct {
	Token      string          `json:"token"`
	Characters []authCharacter `json:"characters"`
}

type legacyRegisterPayload struct {
	Token string `json:"token"`
}

type legacyCharacterIDPayload struct {
	ID uint32 `json:"id"`
}

type legacyCharacterRequest struct {
	Token     string `json:"token"`
	ID        uint32 `json:"id"`
	CharID    uint32 `json:"charId"`
	CharIDAlt uint32 `json:"char_id"`
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	if a.wrapperConfig.ClientImagesHosting {
		mux.Handle("/ClientImages/", http.StripPrefix("/ClientImages/", clientImagesFileServer(filepath.Join(a.root, clientImagesDirName))))
	} else {
		mux.HandleFunc("/ClientImages/", http.NotFound)
	}
	mux.HandleFunc("/check", a.handlePatchCheck)
	mux.HandleFunc("/launcher", a.handleLauncher)
	mux.HandleFunc("/v2/launcher", a.handleLauncher)
	mux.HandleFunc("/login", a.handleLogin)
	mux.HandleFunc("/v2/login", a.handleLogin)
	mux.HandleFunc("/register", a.handleRegister)
	mux.HandleFunc("/v2/register", a.handleRegister)
	mux.HandleFunc("/character/create", a.handleCharacterCreate)
	mux.HandleFunc("/character/delete", a.handleCharacterDelete)
	mux.HandleFunc("/character/export", a.handleCharacterExport)
	mux.HandleFunc("/version", a.handleVersion)
	mux.HandleFunc("/v2/version", a.handleVersion)
	mux.HandleFunc("/v2/server/status", a.handleServerStatus)
	mux.HandleFunc("/api/dashboard/stats", a.handleDashboardStats)
	mux.HandleFunc("/v2/characters", a.handleCharactersCollection)
	mux.HandleFunc("/v2/characters/", a.handleCharactersItem)
	mux.HandleFunc("/v2/altclient/stats", a.handleAltClientStats)
	mux.HandleFunc("/v2/altclient/characters/", a.handleAltClientCharacterSavedata)
	mux.HandleFunc("/", a.handleCatchAll)
	return mux
}

func clientImagesFileServer(root string) http.Handler {
	fileServer := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Access-Control-Allow-Origin", "*")
		headers.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		headers.Set("Access-Control-Expose-Headers", "ETag, Last-Modified, Cache-Control, Content-Length, Content-Type")
		headers.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		headers.Set("Pragma", "no-cache")
		headers.Set("Expires", "0")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func copyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		switch http.CanonicalHeaderKey(key) {
		case "Content-Length", "Transfer-Encoding", "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Upgrade":
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: code, Message: message})
}

func writeJSONResponse(w http.ResponseWriter, status int, headers http.Header, payload any) {
	copyHeaders(w.Header(), headers)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (a *app) executeUpstreamRequest(ctx context.Context, method string, path string, rawQuery string, body []byte, headers http.Header) (*http.Response, []byte, error) {
	target := *a.upstreamURL
	target.Path = path
	target.RawQuery = rawQuery

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return nil, nil, err
	}
	copyHeaders(req.Header, headers)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return resp, respBody, nil
}

func copyUpstreamResponse(w http.ResponseWriter, resp *http.Response, body []byte) {
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (a *app) loadLauncherConfig(base launcherResponse) launcherResponse {
	configPath, err := resolveConfigPath(a.root, mezeportaConfigFileName, legacyAltClientConfigFileName)
	if err != nil {
		a.logger.Printf("failed to resolve launcher config: %v", err)
		return base
	}
	if configPath == "" {
		return base
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		a.logger.Printf("failed to read %s: %v", configPath, err)
		return base
	}

	out := base
	if err := json.Unmarshal(raw, &out); err != nil {
		a.logger.Printf("failed to parse %s: %v", configPath, err)
		return base
	}
	return out
}

func (a *app) publicBaseURL(r *http.Request) string {
	scheme := "http"
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = fmt.Sprintf("127.0.0.1:%d", a.publicPort)
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func (a *app) legacyLoginNotices() []string {
	if a.baseConfig.HideLoginNotice || len(a.baseConfig.LoginNotices) == 0 {
		return []string{}
	}
	return []string{strings.Join(a.baseConfig.LoginNotices, "<PAGE>")}
}

func (a *app) wrapModernAuthPayload(ctx context.Context, payload authData, publicBase string) (authData, error) {
	friends, err := a.altLauncherFriendsForCharacters(ctx, payload.Characters)
	if err != nil {
		return payload, err
	}
	if friends == nil {
		friends = []altLauncherFriend{}
	}
	payload.Friends = friends
	payload.AltSavedataEnabled = a.wrapperConfig.SaveCacheFetch
	if strings.TrimSpace(payload.PatchServer) == "" {
		payload.PatchServer = a.defaultPatchServer(publicBase)
	}
	return payload, nil
}

func (a *app) buildLegacyAuthPayload(ctx context.Context, token string, characters []authCharacter, publicBase string) (authData, error) {
	authRow, err := a.legacyAuthDataForToken(ctx, token)
	if err != nil {
		return authData{}, err
	}
	if characters == nil {
		characters, err = a.legacyCharactersForUser(ctx, authRow.UserID)
		if err != nil {
			return authData{}, err
		}
	}
	markReturning(characters)
	friends, err := a.altLauncherFriendsForCharacters(ctx, characters)
	if err != nil {
		return authData{}, err
	}
	if friends == nil {
		friends = []altLauncherFriend{}
	}

	currentTS := uint32(time.Now().Unix())
	expiryTS := currentTS
	if authRow.ReturnExpires.Valid {
		expiryTS = uint32(authRow.ReturnExpires.Time.Unix())
	}

	return authData{
		CurrentTS:          currentTS,
		ExpiryTS:           expiryTS,
		EntranceCount:      1,
		Notices:            a.legacyLoginNotices(),
		User:               authUser{TokenID: authRow.UserID, Token: token, Rights: authRow.Rights},
		Characters:         characters,
		Courses:            legacyCourseListFromRights(authRow.Rights),
		MezFes:             a.legacyMezFes(),
		Friends:            friends,
		PatchServer:        a.defaultPatchServer(publicBase),
		AltSavedataEnabled: a.wrapperConfig.SaveCacheFetch,
	}, nil
}

func normalizeLegacyCharacterID(req legacyCharacterRequest) uint32 {
	if req.CharID != 0 {
		return req.CharID
	}
	if req.CharIDAlt != 0 {
		return req.CharIDAlt
	}
	return req.ID
}

func (a *app) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	writeJSONResponse(w, http.StatusOK, http.Header{}, versionResponse{
		ClientMode: a.resolvedClientMode,
		Name:       "Erupe-CE",
	})
}

func (a *app) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	status, err := a.legacyServerStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSONResponse(w, http.StatusOK, http.Header{}, status)
}

func (a *app) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if !a.isLegacyLayout() && !a.is93Beta() {
		resp, body, err := a.executeUpstreamRequest(r.Context(), r.Method, r.URL.Path, r.URL.RawQuery, nil, r.Header)
		if err != nil {
			writeError(w, http.StatusBadGateway, "upstream_unavailable", "Upstream API is unavailable")
			return
		}
		copyUpstreamResponse(w, resp, body)
		return
	}

	stats, err := a.legacyDashboardStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSONResponse(w, http.StatusOK, http.Header{}, stats)
}

func (a *app) handleLauncher(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	upstreamPath := r.URL.Path
	if a.usesLegacyHTTPRoutes() && r.URL.Path == "/v2/launcher" {
		upstreamPath = "/launcher"
	}

	resp, respBody, err := a.executeUpstreamRequest(r.Context(), r.Method, upstreamPath, r.URL.RawQuery, nil, r.Header)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_unavailable", "Upstream API is unavailable")
		return
	}
	if resp.StatusCode != http.StatusOK {
		copyUpstreamResponse(w, resp, respBody)
		return
	}

	if a.isLegacyLayout() {
		var legacyPayload legacyLauncherPayload
		if err := json.Unmarshal(respBody, &legacyPayload); err != nil {
			copyUpstreamResponse(w, resp, respBody)
			return
		}
		payload := launcherResponse{
			Banners:  []launcherBanner{},
			Messages: []launcherMessage{},
			Links:    []launcherLink{},
		}
		for _, message := range legacyPayload.Important {
			payload.Messages = append(payload.Messages, launcherMessage{Message: message.Message, Date: message.Date, Link: message.Link, Kind: 1})
		}
		for _, message := range legacyPayload.Normal {
			payload.Messages = append(payload.Messages, launcherMessage{Message: message.Message, Date: message.Date, Link: message.Link, Kind: 0})
		}
		payload = a.loadLauncherConfig(payload)
		writeJSONResponse(w, resp.StatusCode, resp.Header, payload)
		return
	}

	var payload launcherResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		copyUpstreamResponse(w, resp, respBody)
		return
	}
	payload = a.loadLauncherConfig(payload)
	writeJSONResponse(w, resp.StatusCode, resp.Header, payload)
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	a.handleAuth(w, r, true)
}

func (a *app) handleRegister(w http.ResponseWriter, r *http.Request) {
	a.handleAuth(w, r, false)
}

func (a *app) handleAuth(w http.ResponseWriter, r *http.Request, isLogin bool) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}

	if isLogin {
		var reqData struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal(body, &reqData); err == nil && strings.TrimSpace(reqData.Username) != "" {
			userID, lookupErr := a.lookupUserIDByUsername(r.Context(), reqData.Username)
			switch {
			case lookupErr == nil:
				isBanned, banExpiry, banErr := a.altClientBanStatus(r.Context(), userID)
				if banErr != nil {
					writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
					return
				}
				if isBanned {
					writeError(w, http.StatusForbidden, "banned", formatAltClientBanMessage(banExpiry))
					return
				}
			case errors.Is(lookupErr, sql.ErrNoRows):
			default:
				writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
				return
			}
		}
	}

	upstreamPath := r.URL.Path
	if a.usesLegacyHTTPRoutes() {
		switch r.URL.Path {
		case "/v2/login":
			upstreamPath = "/login"
		case "/v2/register":
			upstreamPath = "/register"
		}
	}

	resp, respBody, err := a.executeUpstreamRequest(r.Context(), r.Method, upstreamPath, r.URL.RawQuery, body, r.Header)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_unavailable", "Upstream API is unavailable")
		return
	}
	if resp.StatusCode != http.StatusOK {
		copyUpstreamResponse(w, resp, respBody)
		return
	}

	publicBase := a.publicBaseURL(r)
	if a.isLegacyLayout() {
		if isLogin {
			var payload legacyLoginPayload
			if err := json.Unmarshal(respBody, &payload); err != nil {
				copyUpstreamResponse(w, resp, respBody)
				return
			}
			authPayload, err := a.buildLegacyAuthPayload(r.Context(), payload.Token, payload.Characters, publicBase)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
				return
			}
			writeJSONResponse(w, resp.StatusCode, resp.Header, authPayload)
			return
		}

		var payload legacyRegisterPayload
		if err := json.Unmarshal(respBody, &payload); err != nil {
			copyUpstreamResponse(w, resp, respBody)
			return
		}
		authPayload, err := a.buildLegacyAuthPayload(r.Context(), payload.Token, nil, publicBase)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			return
		}
		writeJSONResponse(w, resp.StatusCode, resp.Header, authPayload)
		return
	}

	var payload authData
	if err := json.Unmarshal(respBody, &payload); err != nil {
		copyUpstreamResponse(w, resp, respBody)
		return
	}
	if a.is93Beta() {
		markReturning(payload.Characters)
		if payload.Courses == nil {
			payload.Courses = legacyCourseListFromRights(payload.User.Rights)
		}
	}
	payload, err = a.wrapModernAuthPayload(r.Context(), payload, publicBase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSONResponse(w, resp.StatusCode, resp.Header, payload)
}

func (a *app) authenticateRequest(r *http.Request) (uint32, string, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		return 0, "", errors.New("missing bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if token == "" {
		return 0, "", errors.New("missing bearer token")
	}
	userID, err := a.userIDFromToken(r.Context(), token)
	return userID, token, err
}

func (a *app) handleCharacterCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !a.isLegacyLayout() {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
			return
		}
		resp, respBody, err := a.executeUpstreamRequest(r.Context(), r.Method, r.URL.Path, r.URL.RawQuery, body, r.Header)
		if err != nil {
			writeError(w, http.StatusBadGateway, "upstream_unavailable", "Upstream API is unavailable")
			return
		}
		copyUpstreamResponse(w, resp, respBody)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	var request legacyCharacterRequest
	if err := json.Unmarshal(body, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	if strings.TrimSpace(request.Token) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
		return
	}
	character, statusCode, headers, err := a.createLegacyCharacter(r.Context(), request.Token)
	if err != nil {
		if statusCode != 0 {
			writeError(w, statusCode, "internal_error", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSONResponse(w, http.StatusOK, headers, character)
}

func (a *app) createLegacyCharacter(ctx context.Context, token string) (authCharacter, int, http.Header, error) {
	requestBody, _ := json.Marshal(map[string]string{"token": token})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	resp, respBody, err := a.executeUpstreamRequest(ctx, http.MethodPost, "/character/create", "", requestBody, headers)
	if err != nil {
		return authCharacter{}, http.StatusBadGateway, http.Header{}, err
	}
	if resp.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(respBody))
		if message == "" {
			message = "Upstream API is unavailable"
		}
		return authCharacter{}, resp.StatusCode, resp.Header, errors.New(message)
	}
	var created legacyCharacterIDPayload
	if err := json.Unmarshal(respBody, &created); err != nil {
		return authCharacter{}, http.StatusInternalServerError, resp.Header, err
	}
	userID, err := a.userIDFromToken(ctx, token)
	if err != nil {
		return authCharacter{}, http.StatusUnauthorized, resp.Header, err
	}
	character, err := a.legacyCharacterByID(ctx, userID, created.ID)
	return character, http.StatusOK, resp.Header, err
}

func (a *app) create93BetaCharacter(ctx context.Context, token string) (authCharacter, int, http.Header, error) {
	requestBody, _ := json.Marshal(map[string]string{"token": token})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	resp, respBody, err := a.executeUpstreamRequest(ctx, http.MethodPost, "/character/create", "", requestBody, headers)
	if err != nil {
		return authCharacter{}, http.StatusBadGateway, http.Header{}, err
	}
	if resp.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(respBody))
		if message == "" {
			message = "Upstream API is unavailable"
		}
		return authCharacter{}, resp.StatusCode, resp.Header, errors.New(message)
	}
	var character authCharacter
	if err := json.Unmarshal(respBody, &character); err != nil {
		return authCharacter{}, http.StatusInternalServerError, resp.Header, err
	}
	return character, http.StatusOK, resp.Header, nil
}

func (a *app) delete93BetaCharacter(ctx context.Context, token string, charID uint32) (int, http.Header, error) {
	requestBody, _ := json.Marshal(map[string]any{"token": token, "charId": charID})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	resp, respBody, err := a.executeUpstreamRequest(ctx, http.MethodPost, "/character/delete", "", requestBody, headers)
	if err != nil {
		return http.StatusBadGateway, http.Header{}, err
	}
	if resp.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(respBody))
		if message == "" {
			message = "Upstream API is unavailable"
		}
		return resp.StatusCode, resp.Header, errors.New(message)
	}
	return http.StatusOK, resp.Header, nil
}

func (a *app) handleCharacterDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !a.isLegacyLayout() {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
			return
		}
		resp, respBody, err := a.executeUpstreamRequest(r.Context(), r.Method, r.URL.Path, r.URL.RawQuery, body, r.Header)
		if err != nil {
			writeError(w, http.StatusBadGateway, "upstream_unavailable", "Upstream API is unavailable")
			return
		}
		copyUpstreamResponse(w, resp, respBody)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	var request legacyCharacterRequest
	if err := json.Unmarshal(body, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	charID := normalizeLegacyCharacterID(request)
	statusCode, headers, err := a.deleteLegacyCharacter(r.Context(), request.Token, charID)
	if err != nil {
		writeError(w, statusCode, "internal_error", err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, headers, map[string]any{})
}

func (a *app) deleteLegacyCharacter(ctx context.Context, token string, charID uint32) (int, http.Header, error) {
	requestBody, _ := json.Marshal(map[string]any{"token": token, "id": charID})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	resp, respBody, err := a.executeUpstreamRequest(ctx, http.MethodPost, "/character/delete", "", requestBody, headers)
	if err != nil {
		return http.StatusBadGateway, http.Header{}, err
	}
	if resp.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(respBody))
		if message == "" {
			message = "Upstream API is unavailable"
		}
		return resp.StatusCode, resp.Header, errors.New(message)
	}
	return http.StatusOK, resp.Header, nil
}

func (a *app) handleCharacterExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	var userID uint32
	var charID uint32
	var err error
	if r.Method == http.MethodGet {
		userID, _, err = a.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
		charID64, _ := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("id")), 10, 32)
		charID = uint32(charID64)
	} else {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
			return
		}
		var request legacyCharacterRequest
		if err := json.Unmarshal(body, &request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
			return
		}
		charID = normalizeLegacyCharacterID(request)
		userID, err = a.userIDFromToken(r.Context(), request.Token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
	}
	if charID == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid character ID")
		return
	}
	character, err := a.exportCharacterData(r.Context(), userID, charID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Character not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSONResponse(w, http.StatusOK, http.Header{}, character)
}

func (a *app) handleCharactersCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !a.isLegacyLayout() && !a.is93Beta() {
			a.proxy.ServeHTTP(w, r)
			return
		}
		userID, _, err := a.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
		var characters []authCharacter
		if a.is93Beta() {
			characters, err = a.modernCharactersForUser(r.Context(), userID)
		} else {
			characters, err = a.legacyCharactersForUser(r.Context(), userID)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			return
		}
		writeJSONResponse(w, http.StatusOK, http.Header{}, characters)
	case http.MethodPost:
		if !a.isLegacyLayout() && !a.is93Beta() {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
				return
			}
			resp, respBody, err := a.executeUpstreamRequest(r.Context(), r.Method, r.URL.Path, r.URL.RawQuery, body, r.Header)
			if err != nil {
				writeError(w, http.StatusBadGateway, "upstream_unavailable", "Upstream API is unavailable")
				return
			}
			copyUpstreamResponse(w, resp, respBody)
			return
		}
		_, token, err := a.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
		var character authCharacter
		var statusCode int
		var headers http.Header
		if a.is93Beta() {
			character, statusCode, headers, err = a.create93BetaCharacter(r.Context(), token)
		} else {
			character, statusCode, headers, err = a.createLegacyCharacter(r.Context(), token)
		}
		if err != nil {
			writeError(w, statusCode, "internal_error", err.Error())
			return
		}
		writeJSONResponse(w, http.StatusOK, headers, character)
	default:
		http.NotFound(w, r)
	}
}

func (a *app) handleCharactersItem(w http.ResponseWriter, r *http.Request) {
	prefix := "/v2/characters/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == r.URL.Path || strings.TrimSpace(rest) == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	charID64, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil || charID64 == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid character ID")
		return
	}
	charID := uint32(charID64)

	suffix := ""
	if len(parts) > 1 {
		suffix = parts[1]
	}

	switch {
	case suffix == "export" && r.Method == http.MethodGet:
		userID, _, err := a.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
		character, err := a.exportCharacterData(r.Context(), userID, charID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Character not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			return
		}
		writeJSONResponse(w, http.StatusOK, http.Header{}, character)
	case suffix == "delete" && (r.Method == http.MethodDelete || r.Method == http.MethodPost):
		if !a.isLegacyLayout() && !a.is93Beta() {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
				return
			}
			resp, respBody, err := a.executeUpstreamRequest(r.Context(), r.Method, r.URL.Path, r.URL.RawQuery, body, r.Header)
			if err != nil {
				writeError(w, http.StatusBadGateway, "upstream_unavailable", "Upstream API is unavailable")
				return
			}
			copyUpstreamResponse(w, resp, respBody)
			return
		}
		_, token, err := a.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
		var statusCode int
		var headers http.Header
		if a.is93Beta() {
			statusCode, headers, err = a.delete93BetaCharacter(r.Context(), token, charID)
		} else {
			statusCode, headers, err = a.deleteLegacyCharacter(r.Context(), token, charID)
		}
		if err != nil {
			writeError(w, statusCode, "internal_error", err.Error())
			return
		}
		writeJSONResponse(w, http.StatusOK, headers, map[string]any{})
	case suffix == "" && r.Method == http.MethodDelete:
		if !a.isLegacyLayout() && !a.is93Beta() {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
				return
			}
			resp, respBody, err := a.executeUpstreamRequest(r.Context(), r.Method, r.URL.Path, r.URL.RawQuery, body, r.Header)
			if err != nil {
				writeError(w, http.StatusBadGateway, "upstream_unavailable", "Upstream API is unavailable")
				return
			}
			copyUpstreamResponse(w, resp, respBody)
			return
		}
		_, token, err := a.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
		var statusCode int
		var headers http.Header
		if a.is93Beta() {
			statusCode, headers, err = a.delete93BetaCharacter(r.Context(), token, charID)
		} else {
			statusCode, headers, err = a.deleteLegacyCharacter(r.Context(), token, charID)
		}
		if err != nil {
			writeError(w, statusCode, "internal_error", err.Error())
			return
		}
		writeJSONResponse(w, http.StatusOK, headers, map[string]any{})
	case suffix == "" && r.Method == http.MethodGet:
		if !a.isLegacyLayout() && !a.is93Beta() {
			a.proxy.ServeHTTP(w, r)
			return
		}
		userID, _, err := a.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
		var character authCharacter
		if a.is93Beta() {
			character, err = a.modernCharacterByID(r.Context(), userID, charID)
		} else {
			character, err = a.legacyCharacterByID(r.Context(), userID, charID)
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Character not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			return
		}
		writeJSONResponse(w, http.StatusOK, http.Header{}, character)
	default:
		http.NotFound(w, r)
	}
}

func (a *app) handleAltClientStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	userID, _, err := a.authenticateRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
		return
	}

	userStats, err := a.altClientUserStats(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	charRows, err := a.altClientCharacterRows(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	charStats := make([]altClientCharacterStats, 0, len(charRows))
	for _, row := range charRows {
		unclaimedDistributions := []altClientDistribution{}
		unclaimedDistributionTotal := uint32(0)
		if a.wrapperConfig.DistributionFetch {
			unclaimedDistributions, unclaimedDistributionTotal, err = a.altClientUnclaimedDistributions(r.Context(), row.ID, 6, 0)
			if err != nil {
				if !a.isLegacyLayout() && !a.is93Beta() {
					writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
					return
				}
				a.logger.Printf("legacy alt-client distributions failed for char %d: %v", row.ID, err)
				unclaimedDistributions = []altClientDistribution{}
				unclaimedDistributionTotal = 0
			}
		}

		unreadMails := []altClientMail{}
		if a.wrapperConfig.MailFetch {
			unreadMails, err = a.altClientUnreadMails(r.Context(), row.ID)
			if err != nil {
				if !a.isLegacyLayout() && !a.is93Beta() {
					writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
					return
				}
				a.logger.Printf("legacy alt-client unread mail failed for char %d: %v", row.ID, err)
				unreadMails = []altClientMail{}
			}
		}

		distNames := make([]string, 0, len(unclaimedDistributions))
		for _, dist := range unclaimedDistributions {
			distNames = append(distNames, dist.EventName)
		}

		charStats = append(charStats, altClientCharacterStats{
			ID:                           row.ID,
			TimePlayed:                   row.TimePlayed,
			UnreadMail:                   uint32(len(unreadMails)),
			UnreadMailEntries:            unreadMails,
			UnclaimedDistributions:       unclaimedDistributionTotal,
			UnclaimedDistributionNames:   distNames,
			UnclaimedDistributionDetails: unclaimedDistributions,
		})
	}

	onlineFriends, err := a.altClientOnlineFriends(r.Context(), charRows)
	if err != nil {
		if !a.isLegacyLayout() && !a.is93Beta() {
			writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			return
		}
		a.logger.Printf("legacy alt-client online friends failed: %v", err)
		onlineFriends = []altClientOnlineFriend{}
	}

	writeJSONResponse(w, http.StatusOK, http.Header{}, altClientStatsResponse{
		User:          userStats,
		Characters:    charStats,
		OnlineFriends: onlineFriends,
	})
}

func parseAltClientCharacterRouteID(path string, suffix string) (uint32, bool) {
	prefix := "/v2/altclient/characters/"
	charIDRaw := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	charIDRaw = strings.Trim(charIDRaw, "/")
	if charIDRaw == "" {
		return 0, false
	}
	charID64, err := strconv.ParseUint(charIDRaw, 10, 32)
	if err != nil || charID64 == 0 {
		return 0, false
	}
	return uint32(charID64), true
}

func parseAltClientDistributionPageValue(raw string, fallback uint32, maxValue uint32) uint32 {
	if raw == "" {
		return fallback
	}
	value64, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return fallback
	}
	value := uint32(value64)
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func (a *app) handleAltClientCharacterDistributions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if !a.wrapperConfig.DistributionFetch {
		http.NotFound(w, r)
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/distributions") {
		http.NotFound(w, r)
		return
	}

	charID, ok := parseAltClientCharacterRouteID(r.URL.Path, "/distributions")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid character ID")
		return
	}

	userID, _, err := a.authenticateRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
		return
	}

	charRows, err := a.altClientCharacterRows(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	characterBelongsToUser := false
	for _, row := range charRows {
		if row.ID == charID {
			characterBelongsToUser = true
			break
		}
	}
	if !characterBelongsToUser {
		writeError(w, http.StatusNotFound, "not_found", "Character not found")
		return
	}

	limit := parseAltClientDistributionPageValue(r.URL.Query().Get("limit"), 6, 6)
	offset := parseAltClientDistributionPageValue(r.URL.Query().Get("offset"), 0, 0)
	entries, total, err := a.altClientUnclaimedDistributions(r.Context(), charID, limit, offset)
	if err != nil {
		if !a.isLegacyLayout() && !a.is93Beta() {
			writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			return
		}
		a.logger.Printf("legacy alt-client distributions failed for char %d: %v", charID, err)
		entries = []altClientDistribution{}
		total = 0
	}

	writeJSONResponse(w, http.StatusOK, http.Header{}, altClientDistributionPageResponse{
		CharacterID: charID,
		Offset:      offset,
		Limit:       limit,
		Total:       total,
		Entries:     entries,
	})
}

func (a *app) handleAltClientCharacterSavedata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/distributions") {
		a.handleAltClientCharacterDistributions(w, r)
		return
	}
	if !a.wrapperConfig.SaveCacheFetch {
		http.NotFound(w, r)
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/savedata") {
		http.NotFound(w, r)
		return
	}

	userID, _, err := a.authenticateRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
		return
	}

	charID, ok := parseAltClientCharacterRouteID(r.URL.Path, "/savedata")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid character ID")
		return
	}

	savedata, err := a.altClientCharacterSavedata(r.Context(), userID, charID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Character not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	writeJSONResponse(w, http.StatusOK, http.Header{}, altClientSavedataResponse{
		CharacterID: charID,
		Savedata:    base64.StdEncoding.EncodeToString(savedata),
		ClientMode:  a.resolvedClientMode,
	})
}

func (a *app) handleCatchAll(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if a.tryServePatchFile(w, r) {
			return
		}
	}
	a.proxy.ServeHTTP(w, r)
}
