package embeddings

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEmbed_ConcurrencySerialized(t *testing.T) {
	var callCount int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2],"index":0}]}`))
	}))
	defer srv.Close()

	p := newOpenAICompatible(srv.URL, "test-model", "", 32)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := p.Embed(context.Background(), []string{"test"})
			if err != nil {
				t.Errorf("Embed failed: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if callCount != 3 {
		t.Errorf("expected 3 API calls, got %d", callCount)
	}
}

func TestEmbed_BusyReturnsError(t *testing.T) {
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-handlerDone:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2],"index":0}]}`))
	}))
	defer func() {
		close(handlerDone)
		srv.Close()
	}()

	p := newOpenAICompatible(srv.URL, "test-model", "", 32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		_, _ = p.Embed(ctx, []string{"first call - long running"})
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)

	_, err := p.Embed(context.Background(), []string{"second call - should get busy error"})
	if err == nil {
		t.Fatal("expected busy error, got nil")
	}
	if !strings.Contains(err.Error(), "busy") {
		t.Errorf("expected 'busy' in error, got: %v", err)
	}

	cancel()
	<-done
}
