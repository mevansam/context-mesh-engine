// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package ttlcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCache_TTL(t *testing.T) {
	var n atomic.Int32
	c := New(80*time.Millisecond, func(context.Context, string) (int, error) {
		return int(n.Add(1)), nil
	})
	if v, err := c.Get(context.Background(), "k"); err != nil || v != 1 {
		t.Fatalf("first = %d %v", v, err)
	}
	if v, err := c.Get(context.Background(), "k"); err != nil || v != 1 {
		t.Fatalf("cached = %d %v", v, err)
	}
	time.Sleep(100 * time.Millisecond)
	if v, err := c.Get(context.Background(), "k"); err != nil || v != 2 {
		t.Fatalf("after TTL = %d %v", v, err)
	}
}

func TestCache_AlwaysRefresh(t *testing.T) {
	var n atomic.Int32
	c := New(-1, func(context.Context, string) (int, error) {
		return int(n.Add(1)), nil
	})
	c.Get(context.Background(), "k")
	c.Get(context.Background(), "k")
	if n.Load() != 2 {
		t.Fatalf("n = %d, want 2", n.Load())
	}
}

func TestCache_ErrorKeepsStale(t *testing.T) {
	var n atomic.Int32
	var fail atomic.Bool
	c := New(time.Millisecond, func(context.Context, string) (int, error) {
		n.Add(1)
		if fail.Load() {
			return 0, errors.New("down")
		}
		return 7, nil
	})
	if v, err := c.Get(context.Background(), "k"); err != nil || v != 7 {
		t.Fatalf("ok = %d %v", v, err)
	}
	time.Sleep(5 * time.Millisecond)
	fail.Store(true)
	v, err := c.Get(context.Background(), "k")
	if err == nil || v != 7 {
		t.Fatalf("stale = %d %v", v, err)
	}
}

func TestCache_ErrorNoStale(t *testing.T) {
	c := New(time.Minute, func(context.Context, string) (int, error) {
		return 0, errors.New("down")
	})
	v, err := c.Get(context.Background(), "k")
	if err == nil || v != 0 {
		t.Fatalf("got %d %v", v, err)
	}
}

func TestCache_Singleflight(t *testing.T) {
	var n atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	c := New(time.Minute, func(context.Context, string) (int, error) {
		n.Add(1)
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		return 1, nil
	})
	var wg sync.WaitGroup
	wg.Add(2)
	run := func() {
		defer wg.Done()
		v, err := c.Get(context.Background(), "k")
		if err != nil || v != 1 {
			t.Errorf("got %d %v", v, err)
		}
	}
	go run()
	<-started
	go run()
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if n.Load() != 1 {
		t.Fatalf("loads = %d, want 1", n.Load())
	}
}
