package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type patchSlotInfo struct {
	queued   bool
	position int
}

type patchWaiter struct {
	ready    chan struct{}
	position int
}

type patchQueueManager struct {
	mu       sync.Mutex
	capacity int
	active   int
	waiters  []*patchWaiter
}

func newPatchQueueManager(capacity int) *patchQueueManager {
	if capacity <= 0 {
		capacity = 1
	}
	return &patchQueueManager{capacity: capacity}
}

func (m *patchQueueManager) acquire(ctx context.Context) (patchSlotInfo, bool) {
	m.mu.Lock()
	if m.active < m.capacity && len(m.waiters) == 0 {
		m.active++
		m.mu.Unlock()
		return patchSlotInfo{}, true
	}

	waiter := &patchWaiter{
		ready:    make(chan struct{}),
		position: len(m.waiters) + 1,
	}
	m.waiters = append(m.waiters, waiter)
	m.mu.Unlock()

	select {
	case <-waiter.ready:
		return patchSlotInfo{queued: true, position: waiter.position}, true
	case <-ctx.Done():
		m.mu.Lock()
		idx := -1
		for i, queued := range m.waiters {
			if queued == waiter {
				idx = i
				break
			}
		}
		if idx >= 0 {
			m.waiters = append(m.waiters[:idx], m.waiters[idx+1:]...)
			for i := idx; i < len(m.waiters); i++ {
				m.waiters[i].position = i + 1
			}
			m.mu.Unlock()
			return patchSlotInfo{}, false
		}
		m.mu.Unlock()
		m.release()
		return patchSlotInfo{}, false
	}
}

func (m *patchQueueManager) release() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.waiters) > 0 {
		waiter := m.waiters[0]
		m.waiters = m.waiters[1:]
		for i := range m.waiters {
			m.waiters[i].position = i + 1
		}
		close(waiter.ready)
		return
	}

	if m.active > 0 {
		m.active--
	}
}

func applyPatchQueueHeaders(w http.ResponseWriter, info patchSlotInfo) {
	if !info.queued || info.position <= 0 {
		return
	}
	w.Header().Set("X-AltClient-Patch-Queued", "1")
	w.Header().Set("X-AltClient-Patch-Queue-Position", strconv.Itoa(info.position))
}

func buildPatchManifest(root string) (string, []byte, error) {
	lines := make([]string, 0)

	err := filepath.WalkDir(root, func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || strings.HasSuffix(filePath, ".gitkeep") {
			return nil
		}

		f, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		checksum := hex.EncodeToString(h.Sum(nil))

		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		rel = strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "/")
		lines = append(lines, fmt.Sprintf("%s\t/%s\n", checksum, rel))
		return nil
	})
	if err != nil {
		return "", nil, err
	}

	sort.Strings(lines)

	manifestHasher := sha256.New()
	var body strings.Builder
	for _, line := range lines {
		_, _ = body.WriteString(line)
		_, _ = manifestHasher.Write([]byte(line))
	}

	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(manifestHasher.Sum(nil)))
	return etag, []byte(body.String()), nil
}

func (a *app) withPatchSlot(w http.ResponseWriter, r *http.Request, fn func(info patchSlotInfo) bool) bool {
	info, ok := a.patchQueue.acquire(r.Context())
	if !ok {
		return false
	}
	defer a.patchQueue.release()
	applyPatchQueueHeaders(w, info)
	return fn(info)
}

func (a *app) currentPatchRoot() (string, error) {
	return resolvePatchRoot(a.root, a.resolvedClientMode)
}

func (a *app) hasPatchRoot() bool {
	root, err := a.currentPatchRoot()
	if err != nil {
		return false
	}

	st, err := os.Stat(root)
	return err == nil && st.IsDir()
}

func (a *app) defaultPatchServer(publicBase string) string {
	if strings.TrimSpace(publicBase) == "" {
		return ""
	}
	return publicBase
}

func writePatchManifestResponse(w http.ResponseWriter, r *http.Request, etag string, body []byte) bool {
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return true
}

func (a *app) handlePatchCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	_ = a.withPatchSlot(w, r, func(info patchSlotInfo) bool {
		root, err := a.currentPatchRoot()
		if err != nil {
			http.NotFound(w, r)
			return true
		}

		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			http.NotFound(w, r)
			return true
		}

		etag, body, err := buildPatchManifest(root)
		if err != nil {
			a.logger.Printf("failed to build patch manifest from %s: %v", root, err)
			http.Error(w, "patch-server-error", http.StatusInternalServerError)
			return true
		}

		return writePatchManifestResponse(w, r, etag, body)
	})
}

func (a *app) tryServePatchFile(w http.ResponseWriter, r *http.Request) bool {
	root, err := a.currentPatchRoot()
	if err != nil {
		return false
	}

	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return false
	}

	cleanPath := path.Clean("/" + r.URL.Path)
	if cleanPath == "/" {
		return false
	}

	rel := strings.TrimPrefix(cleanPath, "/")
	candidate := filepath.Join(root, filepath.FromSlash(rel))
	relToRoot, err := filepath.Rel(root, candidate)
	if err != nil || strings.HasPrefix(relToRoot, "..") {
		return false
	}

	st, err = os.Stat(candidate)
	if err != nil || st.IsDir() {
		return false
	}

	return a.withPatchSlot(w, r, func(info patchSlotInfo) bool {
		http.ServeFile(w, r, candidate)
		return true
	})
}
