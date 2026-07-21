package main

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEnvIntOr(t *testing.T) {
	tests := []struct {
		name     string
		envSet   bool
		envValue string
		def      int
		expected int
	}{
		{"unset returns default", false, "", 42, 42},
		{"valid int overrides default", true, "7", 42, 7},
		{"unparseable returns default", true, "not-a-number", 42, 42},
		{"empty string returns default", true, "", 42, 42},
		{"negative int", true, "-5", 42, -5},
	}

	const key = "YARDSTICK_TEST_ENV_INT_OR"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envSet {
				t.Setenv(key, tt.envValue)
			} else {
				if orig, ok := os.LookupEnv(key); ok {
					t.Cleanup(func() { os.Setenv(key, orig) })
				}
				os.Unsetenv(key)
			}
			result := envIntOr(key, tt.def)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsLifecycleMethod(t *testing.T) {
	for _, method := range []string{methodInitialize, methodPing, methodDiscover, notificationInitialized} {
		assert.True(t, isLifecycleMethod(method), "%s should be a lifecycle method", method)
	}
	for _, method := range []string{"tools/call", "tools/list", ""} {
		assert.False(t, isLifecycleMethod(method), "%s should not be a lifecycle method", method)
	}
}

func TestCounterState_InitializeAndPingNeverCount(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		hangAfter  int
		crashAfter int
		method     string
	}{
		{"initialize in hang mode with hangAfter 1", modeHang, 1, 0, methodInitialize},
		{"ping in hang mode with hangAfter 1", modeHang, 1, 0, methodPing},
		{"discover in hang mode with hangAfter 1", modeHang, 1, 0, methodDiscover},
		{"notifications/initialized in hang mode with hangAfter 1", modeHang, 1, 0, notificationInitialized},
		{"initialize in crash mode with crashAfter 1", modeCrash, 0, 1, methodInitialize},
		{"ping in crash mode with crashAfter 1", modeCrash, 0, 1, methodPing},
		{"discover in crash mode with crashAfter 1", modeCrash, 0, 1, methodDiscover},
		{"notifications/initialized in crash mode with crashAfter 1", modeCrash, 0, 1, notificationInitialized},
		{"initialize in echo mode", "echo", 0, 0, methodInitialize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &counterState{mode: tt.mode, hangAfter: tt.hangAfter, crashAfter: tt.crashAfter}
			for i := 0; i < 5; i++ {
				got := c.decide(tt.method)
				assert.Equal(t, decisionNormal, got)
			}
			assert.Equal(t, 0, c.count)
		})
	}
}

func TestCounterState_HangMode(t *testing.T) {
	c := &counterState{mode: modeHang, hangAfter: 3}

	assert.Equal(t, decisionNormal, c.decide("tools/call"))
	assert.Equal(t, decisionNormal, c.decide("tools/call"))
	assert.Equal(t, decisionHang, c.decide("tools/call"))
}

func TestCounterState_CrashMode(t *testing.T) {
	c := &counterState{mode: modeCrash, crashAfter: 3}

	assert.Equal(t, decisionNormal, c.decide("tools/call"))
	assert.Equal(t, decisionNormal, c.decide("tools/call"))
	assert.Equal(t, decisionCrash, c.decide("tools/call"))
}

func TestCounterState_EchoModeAlwaysNormal(t *testing.T) {
	c := &counterState{mode: "echo"}
	for i := 0; i < 5; i++ {
		assert.Equal(t, decisionNormal, c.decide("tools/call"))
	}
}

// assertNotClosed fails the test if ch has already been closed or has a
// value ready without blocking.
func assertNotClosed(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal(msg)
	default:
	}
}

// assertClosedWithin fails the test if ch does not close within d.
func assertClosedWithin(t *testing.T, ch <-chan struct{}, d time.Duration, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatal(msg)
	}
}

func TestBarrier_ReleasesAtN(t *testing.T) {
	b := &barrier{n: 3, timeout: time.Second}

	ch1 := b.join()
	assertNotClosed(t, ch1, "channel closed after 1 of 3 joins")

	ch2 := b.join()
	assertNotClosed(t, ch1, "channel closed after 2 of 3 joins")
	assertNotClosed(t, ch2, "channel closed after 2 of 3 joins")

	ch3 := b.join()
	assertClosedWithin(t, ch1, 100*time.Millisecond, "ch1 not released after 3rd join")
	assertClosedWithin(t, ch2, 100*time.Millisecond, "ch2 not released after 3rd join")
	assertClosedWithin(t, ch3, 100*time.Millisecond, "ch3 not released after 3rd join")
}

func TestBarrier_SafetyTimeout(t *testing.T) {
	b := &barrier{n: 10, timeout: 20 * time.Millisecond}

	ch1 := b.join()
	ch2 := b.join()

	assertClosedWithin(t, ch1, 200*time.Millisecond, "ch1 not released by safety timeout")
	assertClosedWithin(t, ch2, 200*time.Millisecond, "ch2 not released by safety timeout")
}

func TestBarrier_NIsOneReleasesImmediately(t *testing.T) {
	b := &barrier{n: 1, timeout: time.Second}

	// Every joiner is its own complete window of size 1, so each release
	// should be synchronous, without waiting for a second arrival or the
	// safety timer.
	ch1 := b.join()
	assertClosedWithin(t, ch1, 10*time.Millisecond, "n==1 did not release on the first join")

	ch2 := b.join()
	assertClosedWithin(t, ch2, 10*time.Millisecond, "n==1 did not release on a later join")
}

func TestBarrier_NGreaterThanOneWaitsForFullCount(t *testing.T) {
	b := &barrier{n: 2, timeout: time.Second}

	ch1 := b.join()
	assertNotClosed(t, ch1, "channel closed after only 1 of 2 joins")

	ch2 := b.join()
	assertClosedWithin(t, ch1, 100*time.Millisecond, "window did not release at n")
	assertClosedWithin(t, ch2, 100*time.Millisecond, "window did not release at n")
}

func TestBarrier_ConcurrentJoin(t *testing.T) {
	const n = 20
	b := &barrier{n: n, timeout: time.Second}

	var wg sync.WaitGroup
	results := make(chan (<-chan struct{}), n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- b.join()
		}()
	}
	wg.Wait()
	close(results)

	var chans []<-chan struct{}
	for ch := range results {
		chans = append(chans, ch)
	}
	assert.Len(t, chans, n)

	done := make(chan struct{})
	go func() {
		for _, ch := range chans {
			<-ch
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("not all joiners were released within timeout")
	}
}
