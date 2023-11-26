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
	targets map[string]*targetInfo
	mu      sync.Mutex
	ctx     context.Context
}

func NewManager(ctx context.Context) *Manager {
	return &Manager{
		targets: make(map[string]*targetInfo),
		ctx:     ctx,
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
		// Add a random delay before the initial scrape to distribute the scraping start times
		// nolint:gosec
		initialDelay := time.Duration(rand.Int63n(int64(interval)))
		select {
		case <-time.After(initialDelay):
			// Continue with the scraping after the delay
		case <-scrapeCtx.Done():
			// Context was cancelled during the initial delay
			slog.Info("target scraper cancelled during initial delay", "id", id)
			return
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

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

func (m *Manager) RemoveTarget(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if target, exists := m.targets[id]; exists {
		target.cancel() // Signal the goroutine to stop
		delete(m.targets, id)
	}
	slog.Info("scraping target removed", "id", id)
}
