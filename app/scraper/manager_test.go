package scraper

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Mock scraper function that writes to a channel when called
func mockScrapeFunc(ch chan<- bool) ScrapeFunc {
	return func(ctx context.Context) error {
		ch <- true
		return nil
	}
}

func TestManager_AddTarget(t *testing.T) {
	ctx := Context(t)
	m := NewManager(ctx)
	ch := make(chan bool, 1)
	m.AddTarget("test", mockScrapeFunc(ch), time.Millisecond*50)

	select {
	case <-ch:
	case <-time.After(time.Millisecond * 100):
		t.Error("ScrapeFunc was not called within the expected time")
	}
}

func TestManager_RemoveTarget(t *testing.T) {
	ctx := Context(t)
	m := NewManager(ctx)
	ch := make(chan bool, 1)
	m.AddTarget("test", mockScrapeFunc(ch), time.Millisecond*50)
	<-ch

	m.RemoveTarget("test")
	time.Sleep(time.Millisecond * 100)

	select {
	case <-ch:
		t.Error("ScrapeFunc was called after target was removed")
	default:
	}
}

func TestManager_MultipleTargets(t *testing.T) {
	ctx := Context(t)
	m := NewManager(ctx)

	ch1 := make(chan bool, 1)
	ch2 := make(chan bool, 1)
	ch3 := make(chan bool, 1)

	m.AddTarget("test1", mockScrapeFunc(ch1), time.Millisecond*50)
	m.AddTarget("test2", mockScrapeFunc(ch2), time.Millisecond*60)
	m.AddTarget("test3", mockScrapeFunc(ch3), time.Millisecond*70)

	assert.Eventually(t, func() bool { return <-ch1 }, time.Millisecond*100, time.Millisecond*10)
	assert.Eventually(t, func() bool { return <-ch2 }, time.Millisecond*110, time.Millisecond*10)
	assert.Eventually(t, func() bool { return <-ch3 }, time.Millisecond*120, time.Millisecond*10)

	// Test removing a target and ensuring it doesn't scrape again
	m.RemoveTarget("test2")
	time.Sleep(time.Millisecond * 150)

	select {
	case <-ch2:
		t.Error("ScrapeFunc for test2 was called after target was removed")
	default:
	}

	// Ensure the other targets still scrape
	assert.Eventually(t, func() bool { return <-ch1 }, time.Millisecond*100, time.Millisecond*10)
	assert.Eventually(t, func() bool { return <-ch3 }, time.Millisecond*120, time.Millisecond*10)
}