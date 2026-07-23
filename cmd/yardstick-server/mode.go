package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// envIntOr reads an environment variable and parses it as an int, returning
// def only if the variable is unset. A set-but-unparseable value (e.g. a
// typo'd "3x", or trailing whitespace from a templated manifest) exits the
// process: silently substituting the default would make the requested fault
// fire on the wrong call count (or never), which is exactly the
// misconfiguration validateFaultConfig exists to catch one level down.
func envIntOr(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	// An explicitly empty value (e.g. "BARRIER_N=") is treated as unset;
	// anything else that fails to parse is a typo and fails fast.
	n, err := strconv.Atoi(v)
	if err != nil && v != "" {
		fmt.Fprintf(os.Stderr, "%s must be an integer (got %q)\n", key, v)
		os.Exit(1)
	}
	if err != nil {
		return def
	}
	return n
}

type decision int

const (
	decisionNormal decision = iota
	decisionHang
	decisionCrash
)

const (
	methodInitialize        = "initialize"
	methodPing              = "ping"
	methodDiscover          = "server/discover"
	notificationInitialized = "notifications/initialized"

	modeEcho  = "echo"
	modeHang  = "hang"
	modeCrash = "crash"

	// modeBarrier is handled entirely by the barrier middleware branch; it
	// has no counterState decision since every non-lifecycle call just waits
	// at the barrier.
	modeBarrier = "barrier"
)

// isLifecycleMethod reports whether method is connection setup/handshake
// traffic (rather than a real backend call) and so must never count toward
// hangAfter/crashAfter or join a barrier window. Besides initialize/ping,
// this covers server/discover (sent by Modern clients during connection,
// per SEP-2575) and notifications/initialized (sent by Legacy clients right
// after initialize, per the base MCP spec) - go-sdk routes both through the
// same receiving middleware as any other method.
func isLifecycleMethod(method string) bool {
	switch method {
	case methodInitialize, methodPing, methodDiscover, notificationInitialized:
		return true
	default:
		return false
	}
}

// counterState decides hang/crash behavior based on a running count of
// non-lifecycle method calls (see isLifecycleMethod).
type counterState struct {
	mu         sync.Mutex
	mode       string // modeEcho (default), modeHang, modeCrash; modeBarrier never reaches decide (see newFaultMiddleware)
	hangAfter  int
	crashAfter int
	count      int
}

// decide reports what a handler should do for the given method call.
// Lifecycle methods (see isLifecycleMethod) never count toward
// hangAfter/crashAfter.
func (c *counterState) decide(method string) decision {
	if isLifecycleMethod(method) {
		return decisionNormal
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.count++
	switch {
	case c.mode == modeHang && c.count == c.hangAfter:
		return decisionHang
	case c.mode == modeCrash && c.count == c.crashAfter:
		return decisionCrash
	default:
		return decisionNormal
	}
}

// barrier buffers n arrivals and releases them all at once, or releases
// whoever is waiting early via a safety timer if n is never reached.
type barrier struct {
	mu      sync.Mutex
	n       int
	timeout time.Duration
	win     *barrierWindow
}

type barrierWindow struct {
	release chan struct{}
	count   int
	timer   *time.Timer
}

// join registers one arrival and returns a channel that closes once the
// window fills up (n arrivals) or the safety timer fires, whichever comes
// first.
func (b *barrier) join() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.win == nil {
		w := &barrierWindow{
			release: make(chan struct{}),
			count:   1,
		}
		if w.count >= b.n {
			close(w.release)
			return w.release
		}
		w.timer = time.AfterFunc(b.timeout, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.win == w {
				close(w.release)
				b.win = nil
			}
		})
		b.win = w
		return w.release
	}

	w := b.win
	w.count++
	if w.count >= b.n {
		w.timer.Stop()
		close(w.release)
		b.win = nil
	}
	return w.release
}
