package cursor

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"
)

func TestRefreshTokenDeduplicatesConcurrentExchange(t *testing.T) {
	originalExchange := cursorRefreshExchange
	cursorRefreshGroup = singleflight.Group{}
	cursorRefreshCache = make(map[string]cursorRefreshCacheEntry)
	t.Cleanup(func() {
		cursorRefreshExchange = originalExchange
		cursorRefreshGroup = singleflight.Group{}
		cursorRefreshCache = make(map[string]cursorRefreshCacheEntry)
	})

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	cursorRefreshExchange = func(_ context.Context, token string) (*TokenPair, error) {
		calls.Add(1)
		once.Do(func() { close(started) })
		<-release
		return &TokenPair{AccessToken: "new-access", RefreshToken: token + "-rotated"}, nil
	}

	results := make(chan *TokenPair, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			tokens, err := RefreshToken(context.Background(), "shared-refresh")
			results <- tokens
			errs <- err
		}()
	}

	<-started
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent refresh exchanges=%d, want 1", got)
	}
	close(release)

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		if got := <-results; got == nil || got.AccessToken != "new-access" || got.RefreshToken != "shared-refresh-rotated" {
			t.Fatalf("tokens=%#v", got)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("refresh exchanges=%d, want 1", got)
	}
	if _, err := RefreshToken(context.Background(), "shared-refresh"); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("immediate duplicate exchange count=%d, want cached result", got)
	}
}
