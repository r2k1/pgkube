package scraper

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"
)

type ScrapeFunc func(ctx context.Context) error

type targetInfo struct {
	scrapeFunc ScrapeFunc
	cancel     context.CancelFunc
}

type Manager struct {
	targets              map[string]*targetInfo
	mu                   sync.Mutex
	ctx                  context.Context
	disableScrapingDelay bool
}

func NewManager(ctx context.Context, disableScrapingDelay bool) *Manager {
	return &Manager{
		targets:              make(map[string]*targetInfo),
		ctx:                  ctx,
		disableScrapingDelay: disableScrapingDelay,
	}
}

func (m *Manager) AddTarget(id string, scrapeFunc ScrapeFunc, interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if target already exists
	if _, exists := m.targets[id]; exists {
		return
	}

	scrapeCtx, cancel := context.WithCancel(m.ctx)

	m.targets[id] = &targetInfo{
		scrapeFunc: scrapeFunc,
		cancel:     cancel,
	}

	go func() {
		if !m.disableScrapingDelay {
			randomDelay(scrapeCtx, interval)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// start first scrape immediately
		if err := scrapeFunc(scrapeCtx); err != nil {
			slog.Error("scraping target", "id", id, "error", err)
		}

		for {
			select {
			case <-ticker.C:
				// Perform the scrape
				if err := scrapeFunc(scrapeCtx); err != nil {
					slog.Error("scraping target", "id", id, "error", err)
				}
			case <-scrapeCtx.Done():
				slog.Info("target scraper cancelled", "id", id)
				return
			}
		}
	}()

	slog.Info("new scraping target added", "id", id)
}

func randomDelay(ctx context.Context, maxInterval time.Duration) {
	// nolint:gosec
	initialDelay := time.Duration(rand.Int63n(int64(maxInterval)))
	select {
	case <-time.After(initialDelay):
		// Continue with the scraping after the delay
	case <-ctx.Done():
		// Context was cancelled during the initial delay
		return
	}
}

func (m *Manager) RemoveTarget(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if target, exists := m.targets[id]; exists {
		target.cancel() // Signal the goroutine to stop
		delete(m.targets, id)
	}
	slog.Info("scraping target removed", "id", id)
}
