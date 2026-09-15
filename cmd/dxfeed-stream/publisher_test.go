package main

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	stage2 "github.com/contactkeval/option-replay/internal/pipeline/stage2_dxfeeddatadownloader"
)

// TestPublisherHighFrequencyFeed simulates the live path: a busy event feed
// calling apply+mark continuously while the publisher goroutine drains and
// broadcasts. The real live run stopped emitting after one broadcast, so this
// locks that in. Run with -race to catch shared-state corruption.
func TestPublisherHighFrequencyFeed(t *testing.T) {
	t.Cleanup(func() {
		statBroadcasts.Store(0)
		statFeedEvents.Store(0)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := newState()
	h := newHub()
	h.add() // an SSE client is attached, like the dashboard tab
	pub := newPublisher(h, s)
	go pub.run(ctx)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			s.apply(stage2.LiveEvent{Kind: "Quote", Symbol: "SPX", Bid: 1, Ask: 2})
			pub.mark()
			time.Sleep(100 * time.Microsecond) // ~160 events/s like the live feed
		}
	}()

	time.Sleep(3 * time.Second)
	close(stop)
	wg.Wait()

	got := statBroadcasts.Load()
	if got < 5 {
		t.Fatalf("broadcasts=%d, expected the publisher to keep emitting (~5 Hz over 3s)", got)
	}
	t.Logf("feed happened at %d events/s, publisher emitted %d broadcasts", statFeedEvents.Load()/3, got)

	// dxLink uses NaN/Inf "no value" sentinels for illiquid quotes. A single
	// NaN in a snapshot made json.Marshal fail and silently stall the whole
	// feed (live run showed broadcasts stuck at 1). It must not recur.
	before := statBroadcasts.Load()
	for i := 0; i < 2000; i++ {
		s.apply(stage2.LiveEvent{Kind: "Quote", Symbol: "SPX", Bid: math.NaN(), Ask: math.NaN()})
		s.apply(stage2.LiveEvent{Kind: "Trade", Symbol: "SPX", Price: math.Inf(1)})
		pub.mark()
	}
	time.Sleep(time.Second)
	after := statBroadcasts.Load()
	if after < before+1 {
		t.Fatalf("publisher stalled on NaN inflow: broadcasts before=%d after=%d", before, after)
	}
	t.Logf("broadcasts kept flowing through NaN feed: %d -> %d", before, after)
}
