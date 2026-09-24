package bot

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToLimit(t *testing.T) {
	r := newRateLimiter(time.Second, 3)
	for i := 0; i < 3; i++ {
		if !r.allow(42) {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}
	if r.allow(42) {
		t.Fatal("4th call within window should be denied")
	}
}

func TestRateLimiterPerKeyIsolation(t *testing.T) {
	r := newRateLimiter(time.Second, 1)
	if !r.allow(1) {
		t.Fatal("user 1 first call should pass")
	}
	if !r.allow(2) {
		t.Fatal("user 2 first call should pass (separate key)")
	}
	if r.allow(1) {
		t.Fatal("user 1 second call should be denied")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	r := newRateLimiter(50*time.Millisecond, 1)
	if !r.allow(7) {
		t.Fatal("first call should pass")
	}
	if r.allow(7) {
		t.Fatal("immediate second call should be denied")
	}
	time.Sleep(60 * time.Millisecond)
	if !r.allow(7) {
		t.Fatal("after window expiry call should pass again")
	}
}

func TestCooldown(t *testing.T) {
	c := newCooldown(50 * time.Millisecond)
	if !c.allow(1) {
		t.Fatal("first call should pass")
	}
	if c.allow(1) {
		t.Fatal("call within cooldown should be denied")
	}
	time.Sleep(60 * time.Millisecond)
	if !c.allow(1) {
		t.Fatal("after cooldown call should pass")
	}
}
