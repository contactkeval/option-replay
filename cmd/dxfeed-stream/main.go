package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	stage2 "github.com/contactkeval/option-replay/internal/pipeline/stage2_dxfeeddatadownloader"
)

const (
	streamStep    = 5.0 // SPXW strikes are multiples of 5
	streamRange   = 3   // strikes above/below the ATM anchor
	streamDays    = 2   // current day + next working day
	espInterval   = 200 * time.Millisecond
	reconnectWait = 3 * time.Second
)

var (
	statFeedEvents     atomic.Int64
	statBroadcasts     atomic.Int64
	statSSEFrames      atomic.Int64
	statBroadcastSkips atomic.Int64
)

type quoteState struct {
	Bid, Ask, Last       float64
	BidSize, AskSize     float64
	QuoteTime, TradeTime time.Time
}

func (q *quoteState) mid() float64 {
	if q.Bid > 0 && q.Ask > 0 {
		return (q.Bid + q.Ask) / 2
	}
	return 0
}

type state struct {
	mu sync.RWMutex

	spx        quoteState
	anchor     float64
	strikes    []float64
	expiries   []string // YYMMDD
	expiryISO  []string // yyyy-mm-dd
	calls      []string // ordered option symbols
	puts       []string // ordered option symbols
	quotes     map[string]*quoteState
	connected  bool
	subscribed bool
}

func newState() *state {
	return &state{quotes: make(map[string]*quoteState)}
}

// applySanitize coerces dxFeed NaN/Inf "no value" sentinels to 0 so quotes
// never poison snapshots (json.Marshal rejects NaN/Inf and stalls the feed).
func applySanitize(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func (s *state) apply(ev stage2.LiveEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ev.Symbol == "" {
		return
	}

	switch ev.Kind {
	case "Quote":
		q := s.quoteFor(ev.Symbol)
		q.Bid = applySanitize(ev.Bid)
		q.Ask = applySanitize(ev.Ask)
		q.BidSize = applySanitize(ev.BidSize)
		q.AskSize = applySanitize(ev.AskSize)
		if !ev.Time.IsZero() {
			q.QuoteTime = ev.Time
		}
		if ev.Symbol == "SPX" {
			s.spx.Bid = applySanitize(ev.Bid)
			s.spx.Ask = applySanitize(ev.Ask)
			s.spx.BidSize = applySanitize(ev.BidSize)
			s.spx.AskSize = applySanitize(ev.AskSize)
			if !ev.Time.IsZero() {
				s.spx.QuoteTime = ev.Time
			}
		}
	case "Trade":
		q := s.quoteFor(ev.Symbol)
		q.Last = applySanitize(ev.Price)
		if !ev.Time.IsZero() {
			q.TradeTime = ev.Time
		}
		if ev.Symbol == "SPX" {
			s.spx.Last = applySanitize(ev.Price)
			if !ev.Time.IsZero() {
				s.spx.TradeTime = ev.Time
			}
		}
	}
}

func (s *state) quoteFor(symbol string) *quoteState {
	q, ok := s.quotes[symbol]
	if !ok {
		q = &quoteState{}
		s.quotes[symbol] = q
	}
	return q
}

// spxLevel returns a usable SPX price: last trade, else mid, else bid/ask.
func (s *state) spxLevel() (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v := s.spx.Last; v > 0 {
		return v, true
	}
	if v := s.spx.mid(); v > 0 {
		return v, true
	}
	if s.spx.Bid > 0 {
		return s.spx.Bid, true
	}
	return 0, false
}

func (s *state) setGrid(anchor float64, strikes []float64, expiries []time.Time, base string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.anchor = anchor
	s.strikes = strikes
	s.expiries = s.expiries[:0]
	s.expiryISO = s.expiryISO[:0]
	for _, e := range expiries {
		s.expiries = append(s.expiries, e.Format("060102"))
		s.expiryISO = append(s.expiryISO, e.Format("2006-01-02"))
	}
	s.calls = optionSymbols(base, s.expiries, s.strikes, true)
	s.puts = optionSymbols(base, s.expiries, s.strikes, false)
}

func (s *state) setConnected(v bool) {
	s.mu.Lock()
	s.connected = v
	s.mu.Unlock()
}

func (s *state) markSubscribed(v bool) {
	s.mu.Lock()
	s.subscribed = v
	s.mu.Unlock()
}

// allOptions returns a copy of the subscribed option symbols (calls then puts).
func (s *state) allOptions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.calls)+len(s.puts))
	out = append(out, s.calls...)
	out = append(out, s.puts...)
	return out
}

type snapSPX struct {
	Last      float64 `json:"last"`
	Bid       float64 `json:"bid"`
	Ask       float64 `json:"ask"`
	TradeTime int64   `json:"tradeTime"`
	QuoteTime int64   `json:"quoteTime"`
}

type snapQuote struct {
	Bid   float64 `json:"bid"`
	Ask   float64 `json:"ask"`
	Mid   float64 `json:"mid"`
	Last  float64 `json:"last"`
	T     int64   `json:"t"`
	BSize float64 `json:"bSize"`
	ASize float64 `json:"aSize"`
}

type snapExpiry struct {
	Ymd string `json:"ymd"`
	ISO string `json:"iso"`
}

type snapshot struct {
	Connected bool                 `json:"connected"`
	Anchor    float64              `json:"anchor"`
	Strikes   []float64            `json:"strikes"`
	Expiries  []snapExpiry         `json:"expiries"`
	Calls     []string             `json:"calls"`
	Puts      []string             `json:"puts"`
	SPX       snapSPX              `json:"spx"`
	Quotes    map[string]snapQuote `json:"quotes"`
}

func (s *state) snapshot() *snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := &snapshot{
		Connected: s.connected,
		Anchor:    s.anchor,
		Strikes:   append([]float64(nil), s.strikes...),
		Calls:     append([]string(nil), s.calls...),
		Puts:      append([]string(nil), s.puts...),
		Quotes:    make(map[string]snapQuote, len(s.quotes)),
	}
	for _, e := range s.expiries {
		out.Expiries = append(out.Expiries, snapExpiry{Ymd: e})
	}
	for i, iso := range s.expiryISO {
		if i < len(out.Expiries) {
			out.Expiries[i].ISO = iso
		}
	}
	out.SPX = snapSPX{
		Last: s.spx.Last, Bid: s.spx.Bid, Ask: s.spx.Ask,
		TradeTime: unixSec(s.spx.TradeTime),
		QuoteTime: unixSec(s.spx.QuoteTime),
	}
	for sym, q := range s.quotes {
		out.Quotes[sym] = snapQuote{
			Bid: q.Bid, Ask: q.Ask, Last: q.Last,
			Mid: q.mid(), T: unixSec(q.QuoteTime),
			BSize: q.BidSize, ASize: q.AskSize,
		}
	}
	return out
}

func unixSec(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// hub fans snapshot payloads out to SSE subscribers.
type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newHub() *hub {
	return &hub{clients: make(map[chan []byte]struct{})}
}

func (h *hub) add() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan []byte, 8)
	h.clients[ch] = struct{}{}
	return ch
}

func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func (h *hub) remove(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, ch)
}

func (h *hub) broadcast(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) == 0 {
		statBroadcastSkips.Add(1)
		return
	}
	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
		}
	}
}

// publisher throttles broadcasts to subscribers (max ~5 Hz).
type publisher struct {
	h       *hub
	s       *state
	pending chan struct{}
	done    chan struct{}
}

func newPublisher(h *hub, s *state) *publisher {
	return &publisher{h: h, s: s, pending: make(chan struct{}, 1), done: make(chan struct{})}
}

func (p *publisher) mark() {
	select {
	case p.pending <- struct{}{}:
	default:
	}
}

func (p *publisher) run(ctx context.Context) {
	defer close(p.done)
	ticker := time.NewTicker(espInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case <-p.pending:
				if b, err := json.Marshal(p.s.snapshot()); err == nil {
					p.h.broadcast(b)
					statBroadcasts.Add(1)
				}
			default:
			}
		}
	}
}

func main() {
	listen := flag.String("listen", ":9060", "HTTP listen address")
	webDir := flag.String("web", "./web", "directory with live.html")
	symbol := flag.String("symbol", "SPX", "underlying symbol (index) to track")
	base := flag.String("base", "SPXW", "option root for symbols (.SPXW...C/P)")
	mock := flag.Bool("mock", false, "feed synthetic data (no dxLink connection) for UI testing")
	record := flag.String("record", "", "JSONL file to record live events to (live mode)")
	recordDuration := flag.Duration("record-duration", 60*time.Minute, "auto-stop after this long when recording (0 = until stopped)")
	mockfile := flag.String("mockfile", "", "replay a recorded JSONL file instead of synthetic data (requires -mock)")
	speed := flag.Float64("speed", 1, "replay speed multiplier (with -mockfile)")
	loop := flag.Bool("loop", false, "loop replay until stopped (with -mockfile)")
	flag.Parse()

	if *record != "" && (*mock || *mockfile != "") {
		fmt.Fprintln(os.Stderr, "-record cannot be combined with -mock/-mockfile")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := newState()
	h := newHub()
	pub := newPublisher(h, s)
	go pub.run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		handleSSE(ctx, w, r, h)
	})
	// Serve the dashboard at the root, files under /live.html, /replay.html, etc.
	fileServer := http.FileServer(http.Dir(*webDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/live.html", http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		fileServer.ServeHTTP(w, r)
	})

	server := &http.Server{Addr: *listen, Handler: mux}
	go func() {
		logger.Infof("live dashboard listening on http://localhost%s", *listen)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("http server: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				logger.Infof(
					"stats: feedEvents=%d broadcasts=%d sseFrames=%d broadcastSkips=%d",
					statFeedEvents.Load(), statBroadcasts.Load(),
					statSSEFrames.Load(), statBroadcastSkips.Load(),
				)
			}
		}
	}()

	var rec *recorder
	if *record != "" {
		var err error
		rec, err = newRecorder(*record, *symbol, *base)
		if err != nil {
			logger.Errorf("record open: %v", err)
			os.Exit(1)
		}
		defer rec.close()
		until := time.Until(rec.startAt)
		if until < 0 {
			until = 0
		}
		logger.Infof(
			"recording to %s; window starts at %s (in %s)",
			*record, rec.startAt.Format(time.RFC3339), until.Round(time.Second),
		)
		go func() {
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					rec.flush()
				}
			}
		}()
		if *recordDuration > 0 {
			go func() {
				wait := time.Until(rec.startAt)
				if wait < 0 {
					wait = 0
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
					logger.Infof(
						"recording window start reached (start=%s, running %s)",
						rec.startAt.Format(time.RFC3339), *recordDuration,
					)
				}
				select {
				case <-ctx.Done():
				case <-time.After(*recordDuration):
					logger.Infof("recording duration reached (%s); stopping", *recordDuration)
					stop()
				}
			}()
		}
	}

	cfg := streamConfig{
		symbol: *symbol,
		base:   *base,
		step:   streamStep,
		rangeN: streamRange,
		days:   streamDays,
	}

	if *mock || *mockfile != "" {
		if *mockfile != "" {
			runReplay(ctx, cfg, s, pub, *mockfile, *speed, *loop)
		} else {
			runMock(ctx, cfg, s, pub)
		}
		return
	}

	var opted atomic.Bool
	for {
		err := runStream(ctx, cfg, s, pub, &opted, rec)
		if ctx.Err() != nil {
			logger.Infof("shutting down")
			return
		}
		s.setConnected(false)
		pub.mark()
		opted.Store(false)
		s.markSubscribed(false)
		logger.Warnf("stream ended: %v - reconnecting in %s", err, reconnectWait)
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectWait):
		}
	}
}

type streamConfig struct {
	symbol string
	base   string
	step   float64
	rangeN int
	days   int
}

// runStream connects, subscribes to SPX quotes/trades, then streams events
// until failure or ctx cancellation. The option strike set is computed live
// from the first received SPX level.
func runStream(
	ctx context.Context,
	cfg streamConfig,
	s *state,
	pub *publisher,
	opted *atomic.Bool,
	rec *recorder,
) error {
	client, err := stage2.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	if err := client.Handshake(ctx); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	if err := client.OpenFeedChannel(); err != nil {
		return fmt.Errorf("open feed channel: %w", err)
	}
	if err := client.WaitForChannel(ctx); err != nil {
		return fmt.Errorf("wait feed channel: %w", err)
	}
	client.StartKeepalive(ctx)

	if err := client.SubscribeEvents([]string{cfg.symbol}, "Quote"); err != nil {
		return fmt.Errorf("subscribe %s quote: %w", cfg.symbol, err)
	}
	if err := client.SubscribeEvents([]string{cfg.symbol}, "Trade"); err != nil {
		return fmt.Errorf("subscribe %s trade: %w", cfg.symbol, err)
	}

	s.setConnected(true)
	pub.mark()
	logger.Infof("subscribed to %s Quote+Trade", cfg.symbol)

	handler := func(ev stage2.LiveEvent) error {
		statFeedEvents.Add(1)
		if rec != nil {
			if err := rec.append(ev); err != nil {
				logger.Errorf("record append: %v", err)
			}
		}
		s.apply(ev)
		pub.mark()

		if !opted.Load() {
			if price, ok := s.spxLevel(); ok {
				// Subscribe the option grid off the read loop so a slow write
				// can never stall the stream.
				go subscribeOptions(client, cfg, s, pub, opted, price)
			}
		}
		return nil
	}

	return client.ReadStream(ctx, handler)
}

// computeGrid derives the ATM strike grid and expiry list from a live SPX
// level. Shared by the real subscription path and the mock feed.
func computeGrid(cfg streamConfig, price float64) (float64, []float64, []time.Time) {
	anchor := nearestStep(price, cfg.step)
	strikes := strikesAround(anchor, cfg.step, cfg.rangeN)
	expiries := nextWeekdayDates(time.Now().UTC(), cfg.days)
	return anchor, strikes, expiries
}

// subscribeOptions computes the ATM strike grid from price and subscribes the
// option Quote+Trade streams (once). Runs off the read loop.
func subscribeOptions(
	client *stage2.DXFeedClient,
	cfg streamConfig,
	s *state,
	pub *publisher,
	opted *atomic.Bool,
	price float64,
) {
	if !opted.CompareAndSwap(false, true) {
		return
	}

	anchor, strikes, expiries := computeGrid(cfg, price)
	s.setGrid(anchor, strikes, expiries, cfg.base)

	logger.Infof(
		"SPX level %.2f -> ATM anchor %s strikes=%v expiries=%s/%s",
		price,
		formatStrike(anchor),
		formatStrikes(strikes),
		s.expiries[0], s.expiries[1],
	)

	for _, syms := range [][]string{s.calls, s.puts} {
		if err := client.SubscribeEvents(syms, "Quote"); err != nil {
			logger.Errorf("subscribe option quotes: %v", err)
		}
		if err := client.SubscribeEvents(syms, "Trade"); err != nil {
			logger.Errorf("subscribe option trades: %v", err)
		}
	}
	s.markSubscribed(true)
	pub.mark()
}

func formatStrike(v float64) string {
	return fmt.Sprintf("%d", int(v))
}

func formatStrikes(vs []float64) string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, formatStrike(v))
	}
	return fmt.Sprintf("%v", out)
}

// handleSSE streams an event per connected browser.
func handleSSE(ctx context.Context, w http.ResponseWriter, r *http.Request, h *hub) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := h.add()
	defer h.remove(ch)
	logger.Infof("SSE client connected (%d total)", h.count())
	defer logger.Infof("SSE client disconnected (%d total)", h.count()-1)

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ctx.Done():
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
			statSSEFrames.Add(1)
		}
	}
}

// nearestStep rounds price to the nearest multiple of step (half away from
// zero). Used to anchor the ATM strike on the live SPX price (step = 5).
func nearestStep(price, step float64) float64 {
	return math.Round(price/step) * step
}

// strikesAround returns n strikes below, the anchor, and n strikes above,
// spaced by step. With n=3 and step=5 that is ATM-15 .. ATM+15 (7 strikes).
func strikesAround(anchor, step float64, n int) []float64 {
	out := make([]float64, 0, 2*n+1)
	for i := -n; i <= n; i++ {
		out = append(out, anchor+step*float64(i))
	}
	return out
}

// nextWeekdayDates returns the next n weekdays starting from from (inclusive).
// Weekends are skipped; statutory holidays are not known here.
func nextWeekdayDates(from time.Time, n int) []time.Time {
	out := make([]time.Time, 0, n)
	d := from
	for len(out) < n {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			out = append(out, d)
		}
		d = d.AddDate(0, 0, 1)
	}
	return out
}

// optionSymbols builds dxLink option symbols in the form .SPXW260911C7640.
// base is the option root (e.g. SPXW), expiryYmd is YYMMDD ("260911").
func optionSymbols(base string, expiries []string, strikes []float64, isCall bool) []string {
	kind := "P"
	if isCall {
		kind = "C"
	}
	out := make([]string, 0, len(expiries)*len(strikes))
	for _, expiry := range expiries {
		for _, strike := range strikes {
			out = append(out, fmt.Sprintf(
				".%s%s%s%d",
				base,
				expiry,
				kind,
				int(strike),
			))
		}
	}
	return out
}

// runMock feeds synthetic SPX and option quotes into the same state/publisher
// pipeline so the SSE endpoint and dashboard can be exercised without a
// dxLink connection.
func runMock(ctx context.Context, cfg streamConfig, s *state, pub *publisher) {
	logger.Infof("mock mode: feeding synthetic data for %s/%s", cfg.symbol, cfg.base)
	s.setConnected(true)
	pub.mark()

	var gridDone bool
	start := time.Now()

	update := func() {
		now := time.Now().UTC()
		elapsed := time.Since(start).Seconds()
		price := 7640 + 10*math.Sin(elapsed/25) + 2*math.Sin(elapsed/7)

		s.apply(stage2.LiveEvent{Kind: "Trade", Symbol: cfg.symbol, Price: price, Time: now})
		s.apply(stage2.LiveEvent{
			Kind: "Quote", Symbol: cfg.symbol,
			Bid: price - 0.3, Ask: price + 0.3, BidSize: 1, AskSize: 1, Time: now,
		})
		pub.mark()

		if !gridDone {
			anchor, strikes, expiries := computeGrid(cfg, price)
			s.setGrid(anchor, strikes, expiries, cfg.base)
			gridDone = true
			logger.Infof("mock grid: anchor=%s strikes=%v", formatStrike(anchor), formatStrikes(strikes))
		}

		optionQuotes(price, now, s)
		pub.mark()
	}

	update()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			update()
		}
	}
}

// optionQuotes fills synthetic bid/ask/last for every subscribed option
// symbol, roughly consistent with the underlying level.
func optionQuotes(underlying float64, now time.Time, s *state) {
	for _, sym := range s.allOptions() {
		strike, isCall := parseOptionTail(sym)
		diff := underlying - strike
		var mid float64
		if isCall {
			mid = math.Max(0.10, 30-diff*0.8)
		} else {
			mid = math.Max(0.10, 30+diff*0.8)
		}
		s.apply(stage2.LiveEvent{
			Kind: "Quote", Symbol: sym,
			Bid: mid - 0.15, Ask: mid + 0.15, BidSize: 2, AskSize: 3, Time: now,
		})
		s.apply(stage2.LiveEvent{
			Kind: "Trade", Symbol: sym, Price: mid + 0.05, Time: now,
		})
	}
}

// parseOptionTail extracts strike and call/put kind from a symbol like
// .SPXW260911C7645. Returns 0/false if the symbol is malformed.
func parseOptionTail(sym string) (float64, bool) {
	base := strings.TrimPrefix(strings.TrimPrefix(sym, "."), "SPXW")
	if len(base) < 7 {
		return 0, false
	}
	body := base[6:] // dates are 6 chars: YYMMDD
	var kind byte
	for i := 0; i < len(body); i++ {
		if body[i] == 'C' || body[i] == 'P' {
			kind = body[i]
			body = body[i+1:]
			break
		}
	}
	if kind == 0 || body == "" {
		return 0, false
	}
	var strike float64
	for i := 0; i < len(body); i++ {
		if body[i] < '0' || body[i] > '9' {
			return 0, false
		}
		strike = strike*10 + float64(body[i]-'0')
	}
	return strike, kind == 'C'
}

// nextBoundary returns the next wall-clock multiple of d strictly after t,
// or t itself when already on a boundary. Duration is rounded to the local
// clock (e.g. 10 minutes -> :00/:10/:20 in wall time).
func nextBoundary(t time.Time, d time.Duration) time.Time {
	startOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	el := t.Sub(startOfDay)
	rem := el % d
	if rem == 0 {
		return t
	}
	return t.Add(d - rem)
}

// recorder appends raw LiveEvents to a JSONL file during live runs. Events are
// dropped until startAt, the pre-computed recording window boundary.
type recorder struct {
	mu      sync.Mutex
	f       *os.File
	w       *bufio.Writer
	n       int64
	startAt time.Time
}

func newRecorder(path, symbol, base string) (*recorder, error) {
	startAt := nextBoundary(time.Now(), 10*time.Minute).UTC()
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriter(f)
	header := fmt.Sprintf(
		"{\"meta\":\"dxfeed-recording\",\"version\":1,\"symbol\":%q,\"base\":%q,\"start\":%q}\n",
		symbol, base, startAt.Format(time.RFC3339),
	)
	if _, err := w.WriteString(header); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &recorder{f: f, w: w, startAt: startAt}, nil
}

func (r *recorder) append(ev stage2.LiveEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Now().UTC().Before(r.startAt) {
		return nil // outside the recording window
	}
	if ev.Time.IsZero() {
		// The feed often omits a timestamp for quote/trade events; stamp the
		// arrival time so replays can reproduce the original cadence.
		ev.Time = time.Now().UTC()
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := r.w.Write(append(b, '\n')); err != nil {
		return err
	}
	r.n++
	return nil
}

func (r *recorder) flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.w.Flush()
}

func (r *recorder) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.w.Flush()
	_ = r.f.Sync()
	_ = r.f.Close()
	logger.Infof("recorded %d events to %s", r.n, r.f.Name())
}

// loadRecording reads a JSONL recording produced by recorder, skipping the
// meta header and returning it alongside the events. A recording without any
// meta header yields a zero start time.
func loadRecording(path string) ([]stage2.LiveEvent, time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()

	var events []stage2.LiveEvent
	var start time.Time
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var probe struct {
			Meta  string `json:"meta"`
			Start string `json:"start"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err == nil && probe.Meta != "" {
			if t, err := time.Parse(time.RFC3339, probe.Start); err == nil {
				start = t.UTC()
			}
			continue
		}
		var ev stage2.LiveEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, time.Time{}, fmt.Errorf("bad recording line: %w", err)
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, time.Time{}, err
	}
	return events, start, nil
}

// recordedGrid is the strike/expiry set derived from a recording itself, so a
// replay reproduces exactly the same option symbols regardless of when it runs.
type recordedGrid struct {
	base     string
	expiries []string
	strikes  []float64
	anchor   float64
}

// buildRecordedGrid scans recorded events for option symbols and the first SPX
// level, recovering the symbol space that was live at record time.
func buildRecordedGrid(events []stage2.LiveEvent, step float64) recordedGrid {
	var g recordedGrid
	seenY := map[string]bool{}
	seenS := map[float64]bool{}

	for _, ev := range events {
		if ev.Symbol == "SPX" {
			if g.anchor == 0 {
				if ev.Kind == "Trade" && ev.Price > 0 {
					g.anchor = ev.Price
				} else if ev.Bid > 0 {
					g.anchor = ev.Bid
				}
			}
			continue
		}
		root, ymd, _, strike, ok := parseOptionSymbol(ev.Symbol)
		if !ok {
			continue
		}
		if g.base == "" {
			g.base = root
		}
		if !seenY[ymd] {
			seenY[ymd] = true
			g.expiries = append(g.expiries, ymd)
		}
		if !seenS[strike] {
			seenS[strike] = true
			g.strikes = append(g.strikes, strike)
		}
	}
	sort.Strings(g.expiries)
	sort.Float64s(g.strikes)
	if g.anchor > 0 {
		g.anchor = nearestStep(g.anchor, step)
	}
	return g
}

// parseOptionSymbol splits .SPXW260914C7600 into root "SPXW", ymd "260914",
// kind 'C', and strike 7600.
func parseOptionSymbol(sym string) (root, ymd string, kind byte, strike float64, ok bool) {
	s := strings.TrimPrefix(sym, ".")
	for i := 0; i+6 <= len(s); i++ {
		if !allDigits(s[i : i+6]) {
			continue
		}
		if i+6 < len(s) && (s[i+6] == 'C' || s[i+6] == 'P') {
			tail := s[i+7:]
			var v float64
			for j := 0; j < len(tail) && tail[j] >= '0' && tail[j] <= '9'; j++ {
				v = v*10 + float64(tail[j]-'0')
			}
			if v == 0 {
				return "", "", 0, 0, false
			}
			return s[:i], s[i : i+6], s[i+6], v, true
		}
	}
	return "", "", 0, 0, false
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ymdToTime converts YYMMDD ("260914") to a UTC time.
func ymdToTime(ymd string) (time.Time, bool) {
	if len(ymd) != 6 {
		return time.Time{}, false
	}
	y, err1 := strconv.Atoi("20" + ymd[:2])
	m, err2 := strconv.Atoi(ymd[2:4])
	d, err3 := strconv.Atoi(ymd[4:6])
	if err1 != nil || err2 != nil || err3 != nil || m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), true
}

// runReplay feeds a recorded JSONL file into the same state/publisher pipeline
// at (approximately) the original cadence scaled by speed.
func runReplay(
	ctx context.Context,
	cfg streamConfig,
	s *state,
	pub *publisher,
	path string,
	speed float64,
	loop bool,
) {
	if speed <= 0 {
		speed = 1
	}

	events, startTS, err := loadRecording(path)
	if err != nil {
		logger.Errorf("failed to load recording %s: %v", path, err)
		return
	}
	if len(events) == 0 {
		logger.Warnf("recording %s is empty", path)
		return
	}

	hasTime := false
	for _, ev := range events {
		if !ev.Time.IsZero() {
			hasTime = true
			break
		}
	}
	if !hasTime {
		// Legacy recordings carry no per-event timestamps. Pacing is
		// synthesized by spreading events evenly across the recording's
		// real wall-clock span (header start -> file mtime).
		end := time.Now()
		span := 30 * time.Minute
		if info, err := os.Stat(path); err == nil {
			end = info.ModTime().UTC()
		}
		if !startTS.IsZero() {
			if d := end.Sub(startTS); d > 2*time.Second {
				span = d
			} else {
				startTS = end.Add(-span)
			}
		} else {
			startTS = end.Add(-span)
		}
		n := float64(len(events) - 1)
		for i := range events {
			if n > 0 {
				events[i].Time = startTS.Add(time.Duration(float64(span) * float64(i) / n))
			}
		}
		logger.Infof("recording has no timestamps; pacing %d events across %s", len(events), span.Round(time.Second))
	}

	logger.Infof("replay: %d events from %s (speed x%.2f)", len(events), path, speed)
	s.setConnected(true)
	pub.mark()

	gridSet := false
	if g := buildRecordedGrid(events, cfg.step); g.base != "" && len(g.expiries) > 0 && len(g.strikes) > 0 {
		exps := make([]time.Time, 0, len(g.expiries))
		for _, ymd := range g.expiries {
			if t, ok := ymdToTime(ymd); ok {
				exps = append(exps, t)
			}
		}
		if len(exps) == len(g.expiries) {
			s.setGrid(g.anchor, g.strikes, exps, g.base)
			s.markSubscribed(true)
			gridSet = true
			logger.Infof(
				"replay grid from recording: base=%s anchor=%s strikes=%d expiries=%s",
				g.base, formatStrike(g.anchor), len(g.strikes), strings.Join(g.expiries, ","),
			)
		}
	}

	process := func(ev stage2.LiveEvent) {
		statFeedEvents.Add(1)
		s.apply(ev)
		pub.mark()
		if !gridSet {
			if price, ok := s.spxLevel(); ok {
				anchor, strikes, expiries := computeGrid(cfg, price)
				s.setGrid(anchor, strikes, expiries, cfg.base)
				gridSet = true
				logger.Infof(
					"replay derived grid: anchor=%s strikes=%s",
					formatStrike(anchor), formatStrikes(strikes),
				)
			}
		}
	}

	sortEventsByTime(events)

	var last time.Time
	for {
		for _, ev := range events {
			if ctx.Err() != nil {
				return
			}
			if !last.IsZero() && !ev.Time.IsZero() {
				d := ev.Time.Sub(last)
				if d > 0 {
					delay := time.Duration(float64(d) / speed)
					select {
					case <-ctx.Done():
						return
					case <-time.After(delay):
					}
				}
			}
			if !ev.Time.IsZero() {
				last = ev.Time
			}
			process(ev)
		}
		if !loop {
			break
		}
		last = time.Time{}
		logger.Infof("replay loop restarting")
	}
	logger.Infof("replay finished (%d events replayed)", len(events))
}

// sortEventsByTime orders a recording by event time (files are normally in
// order; this guards against jitter).
func sortEventsByTime(events []stage2.LiveEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})
}
