package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	layouts "lilbutter-wrapper/Layouts"

	"github.com/jmoiron/sqlx"
)

type app struct {
	root               string
	baseConfig         baseConfig
	wrapperConfig      wrapperConfig
	profile            layouts.Profile
	resolvedClientMode string
	publicPort         int
	db                 *sqlx.DB
	hasBansTable       bool
	dbCapabilities     dbCapabilities
	httpClient         *http.Client
	httpServer         *http.Server
	proxy              *httputil.ReverseProxy
	upstreamURL        *url.URL
	upstreamPort       int
	patchQueue         *patchQueueManager
	logger             *log.Logger
	child              *exec.Cmd
	childExit          chan error
	cleanup            func()
}

func newApp(root string) (*app, error) {
	wrapperCfg, err := loadWrapperConfig(root)
	if err != nil {
		return nil, err
	}
	profile, err := resolveWrapperProfile(wrapperCfg)
	if err != nil {
		return nil, err
	}
	baseCfg, err := loadBaseConfig(root)
	if err != nil {
		return nil, err
	}

	publicPort, err := profile.PublicPort(baseCfg)
	if err != nil {
		return nil, err
	}
	resolvedClientMode, err := profile.ResolveClientMode(baseCfg, wrapperCfg.ClientMode92)
	if err != nil {
		return nil, err
	}

	upstreamPort := resolveUpstreamAPIPort(publicPort)
	if upstreamPort == publicPort {
		return nil, fmt.Errorf("wrapper upstream port must differ from public port")
	}

	db, err := openDatabase(baseCfg)
	if err != nil {
		return nil, err
	}

	hasBansTable, err := detectHasBansTable(context.Background(), db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	dbCapabilities, err := detectDBCapabilities(context.Background(), db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	upstreamURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", upstreamPort))
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	a := &app{
		root:               root,
		baseConfig:         baseCfg,
		wrapperConfig:      wrapperCfg,
		profile:            profile,
		resolvedClientMode: resolvedClientMode,
		publicPort:         publicPort,
		db:                 db,
		hasBansTable:       hasBansTable,
		dbCapabilities:     dbCapabilities,
		httpClient:         &http.Client{Timeout: 30 * time.Second},
		upstreamURL:        upstreamURL,
		upstreamPort:       upstreamPort,
		patchQueue:         newPatchQueueManager(wrapperCfg.MaxClientPatch),
		logger:             log.New(os.Stdout, "[MezeportaWrapper] ", log.LstdFlags),
		childExit:          make(chan error, 1),
	}

	a.proxy = httputil.NewSingleHostReverseProxy(upstreamURL)
	a.proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, proxyErr error) {
		a.logger.Printf("proxy error for %s: %v", r.URL.Path, proxyErr)
		writeError(w, http.StatusBadGateway, "upstream_unavailable", "Upstream API is unavailable")
	}
	a.httpServer = &http.Server{Handler: a.routes()}
	return a, nil
}

func (a *app) isLegacyLayout() bool {
	switch a.profile.Name() {
	case "9.2", "9.2.1":
		return true
	default:
		return false
	}
}

// 9.3 beta is intentionally not a legacy database/auth
func (a *app) usesLegacyHTTPRoutes() bool {
	switch a.profile.Name() {
	case "9.2", "9.2.1", "9.3b":
		return true
	default:
		return false
	}
}

func (a *app) is93Beta() bool {
	return a.profile.Name() == "9.3b"
}

func (a *app) publicListenAddr() string {
	return fmt.Sprintf(":%d", a.publicPort)
}

func (a *app) Start() error {
	listener, err := net.Listen("tcp", a.publicListenAddr())
	if err != nil {
		return err
	}

	workspaceDir, cleanup, err := writeDerivedUpstreamConfig(a.root, a.profile, a.wrapperConfig, a.upstreamPort)
	if err != nil {
		_ = listener.Close()
		return err
	}
	a.cleanup = cleanup

	if err := a.startUpstream(workspaceDir); err != nil {
		cleanup()
		a.cleanup = nil
		_ = listener.Close()
		return err
	}

	if err := a.waitForUpstreamReady(15 * time.Second); err != nil {
		a.stopChild()
		cleanup()
		a.cleanup = nil
		_ = listener.Close()
		return err
	}

	go func() {
		if err := a.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Printf("wrapper http server error: %v", err)
		}
	}()

	a.logger.Printf("wrapper listening on %s and proxying upstream on %s using %s layout", a.publicListenAddr(), a.upstreamURL, a.profile.Name())
	return nil
}

func (a *app) startUpstream(workspaceDir string) error {
	upstreamPath := ""
	for _, name := range upstreamExecutableNames() {
		candidate := filepath.Join(a.root, name)
		if _, err := os.Stat(candidate); err == nil {
			upstreamPath = candidate
			break
		}
	}
	if upstreamPath == "" {
		return fmt.Errorf("upstream executable not found beside wrapper; checked %v", upstreamExecutableNames())
	}

	cmd := exec.Command(upstreamPath)
	cmd.Dir = workspaceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return err
	}
	a.child = cmd
	go func() {
		a.childExit <- cmd.Wait()
	}()
	return nil
}

func (a *app) waitForUpstreamReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-a.childExit:
			if err == nil {
				return fmt.Errorf("upstream exited before becoming ready")
			}
			return fmt.Errorf("upstream exited before becoming ready: %w", err)
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		target := *a.upstreamURL
		target.Path = a.profile.ReadyPath()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		resp, err := a.httpClient.Do(req)
		cancel()
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for upstream API on %s", a.upstreamURL)
}

func (a *app) Wait() error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-a.childExit:
		_ = a.shutdownHTTP()
		a.closeResources()
		if err != nil {
			return err
		}
		return nil
	case <-sigCh:
		a.logger.Printf("shutdown requested")
		return a.Shutdown()
	}
}

func (a *app) Shutdown() error {
	httpErr := a.shutdownHTTP()
	childErr := a.stopChild()
	a.closeResources()
	if httpErr != nil {
		return httpErr
	}
	return childErr
}

func (a *app) shutdownHTTP() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.httpServer.Shutdown(ctx)
}

func (a *app) stopChild() error {
	if a.child == nil || a.child.Process == nil {
		return nil
	}

	_ = a.child.Process.Signal(os.Interrupt)
	select {
	case err := <-a.childExit:
		a.child = nil
		return err
	case <-time.After(5 * time.Second):
	}

	killErr := a.child.Process.Kill()
	var waitErr error
	select {
	case waitErr = <-a.childExit:
	case <-time.After(2 * time.Second):
	}
	a.child = nil
	if killErr != nil {
		return killErr
	}
	return waitErr
}

func (a *app) closeResources() {
	if a.db != nil {
		_ = a.db.Close()
		a.db = nil
	}
	if a.cleanup != nil {
		a.cleanup()
		a.cleanup = nil
	}
}
