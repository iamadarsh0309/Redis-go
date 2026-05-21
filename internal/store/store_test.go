package store

import (
	"sync"
	"testing"
	"time"
)

func nowMs() int64 { return time.Now().UnixMilli() }

func TestSetGetDelete(t *testing.T) {
	s := NewMap()
	s.Set("k", NewString("v"))
	if obj, ok := s.Get("k"); !ok || obj.StringVal != "v" {
		t.Fatalf("Get after Set: %+v ok=%v", obj, ok)
	}
	if !s.Delete("k") {
		t.Fatal("Delete on existing returned false")
	}
	if _, ok := s.Get("k"); ok {
		t.Fatal("Get after Delete should miss")
	}
	if s.Delete("k") {
		t.Fatal("Delete on missing returned true")
	}
}

func TestSetResetsExpiry(t *testing.T) {
	s := NewMap()
	s.Set("k", NewString("v"))
	s.SetExpiry("k", nowMs()+10_000)
	s.Set("k", NewString("v2"))
	obj, _ := s.Get("k")
	if obj.ExpiresAt != 0 {
		t.Fatalf("plain SET should clear TTL, got ExpiresAt=%d", obj.ExpiresAt)
	}
}

func TestLazyExpiration(t *testing.T) {
	s := NewMap()
	s.Set("k", NewString("v"))
	s.SetExpiry("k", nowMs()-1)
	if _, ok := s.Get("k"); ok {
		t.Fatal("Get on expired key should miss")
	}
	if s.Len() != 0 {
		t.Fatalf("expired key not deleted by lazy expiration, len=%d", s.Len())
	}
}

func TestActiveSweep(t *testing.T) {
	s := NewMap()
	for i := 0; i < 50; i++ {
		k := string(rune('a' + i%26)) + string(rune('a' + i/26))
		s.Set(k, NewString("v"))
		s.SetExpiry(k, nowMs()-1)
	}
	if s.Len() != 50 {
		t.Fatalf("setup: expected 50 keys, got %d", s.Len())
	}
	evicted := s.Sweep(nowMs(), 1000)
	if evicted != 50 || s.Len() != 0 {
		t.Fatalf("sweep evicted=%d remaining=%d, want 50/0", evicted, s.Len())
	}
}

func TestSweepBoundedBySample(t *testing.T) {
	s := NewMap()
	for i := 0; i < 100; i++ {
		k := string(rune('a'+i%26)) + string(rune('a'+i/26))
		s.Set(k, NewString("v"))
		s.SetExpiry(k, nowMs()-1)
	}
	evicted := s.Sweep(nowMs(), 10)
	if evicted > 10 {
		t.Fatalf("sweep exceeded maxSample: evicted=%d", evicted)
	}
}

func TestPersistClearsExpiry(t *testing.T) {
	s := NewMap()
	s.Set("k", NewString("v"))
	s.SetExpiry("k", nowMs()+10_000)
	if !s.Persist("k") {
		t.Fatal("Persist on key-with-TTL returned false")
	}
	obj, _ := s.Get("k")
	if obj.ExpiresAt != 0 {
		t.Fatal("Persist failed to clear TTL")
	}
	if s.Persist("k") {
		t.Fatal("Persist on key-without-TTL should return false")
	}
}

func TestSetExpiryMissingKey(t *testing.T) {
	s := NewMap()
	if s.SetExpiry("nope", nowMs()+1000) {
		t.Fatal("SetExpiry on missing key returned true")
	}
}

func TestExists(t *testing.T) {
	s := NewMap()
	if s.Exists("missing") {
		t.Fatal("Exists on missing key returned true")
	}
	s.Set("k", NewString("v"))
	if !s.Exists("k") {
		t.Fatal("Exists on present key returned false")
	}
	s.SetExpiry("k", nowMs()-1)
	if s.Exists("k") {
		t.Fatal("Exists on expired key should return false")
	}
}

func TestUpdateCreatesNewKey(t *testing.T) {
	s := NewMap()
	s.Update("h", func(o *Object, present bool) (bool, bool) {
		if present {
			t.Fatal("expected key absent")
		}
		*o = NewHash()
		o.HashVal["f1"] = "v1"
		return true, false
	})
	var got string
	s.View("h", func(o Object, present bool) {
		if !present || o.Type != TypeHash {
			t.Fatal("View after Update did not see new hash")
		}
		got = o.HashVal["f1"]
	})
	if got != "v1" {
		t.Fatalf("HashVal[f1]=%q want v1", got)
	}
}

func TestUpdateDeleteRemovesKey(t *testing.T) {
	s := NewMap()
	s.Set("k", NewString("v"))
	s.Update("k", func(o *Object, present bool) (bool, bool) {
		return false, true
	})
	if s.Exists("k") {
		t.Fatal("Update with delete=true did not remove key")
	}
}

func TestStatsTracksHitsAndMisses(t *testing.T) {
	s := NewMap()
	s.Set("k", NewString("v"))
	s.Get("k")
	s.Get("k")
	s.Get("nope")
	s.Get("nope2")
	st := s.Stats()
	if st.Hits != 2 || st.Misses != 2 {
		t.Fatalf("hits=%d misses=%d (want 2/2)", st.Hits, st.Misses)
	}
	if st.StringKeys != 1 {
		t.Fatalf("StringKeys=%d (want 1)", st.StringKeys)
	}
}

func TestMemoryAccounting(t *testing.T) {
	s := NewMap()
	before := s.Stats().MemoryBytes
	s.Set("k", NewString("hello"))
	after := s.Stats().MemoryBytes
	if after <= before {
		t.Fatalf("MemoryBytes did not grow on Set: before=%d after=%d", before, after)
	}
	s.Delete("k")
	if s.Stats().MemoryBytes != before {
		t.Fatalf("MemoryBytes did not return to baseline after Delete: %d", s.Stats().MemoryBytes)
	}
}

func TestEvictionNoeviction(t *testing.T) {
	s := NewMap()
	s.SetCapacity(2, NoEviction)
	s.Set("a", NewString("1"))
	s.Set("b", NewString("2"))
	s.Set("c", NewString("3"))
	if s.Len() != 3 {
		t.Fatalf("noeviction should allow overage, got Len=%d", s.Len())
	}
}

func TestEvictionAllKeysRandom(t *testing.T) {
	s := NewMap()
	s.SetCapacity(3, AllKeysRandom)
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		s.Set(k, NewString("x"))
	}
	if s.Len() != 3 {
		t.Fatalf("allkeys-random should enforce cap, got Len=%d", s.Len())
	}
	if got := s.Stats().EvictedKeys; got != 2 {
		t.Fatalf("expected 2 evictions, got %d", got)
	}
}

func TestEvictionVolatileTTL(t *testing.T) {
	s := NewMap()
	s.SetCapacity(3, VolatileTTL)
	s.Set("soon", NewString("x"))
	s.SetExpiry("soon", nowMs()+1000)
	s.Set("late", NewString("y"))
	s.SetExpiry("late", nowMs()+100_000)
	s.Set("forever", NewString("z"))
	s.Set("trigger", NewString("t"))
	if s.Exists("soon") {
		t.Errorf("volatile-ttl did not evict soonest-expiring key")
	}
	if s.Len() != 3 {
		t.Fatalf("Len=%d want 3", s.Len())
	}
}

func TestConcurrentSetGet(t *testing.T) {
	s := NewMap()
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				k := string(rune('a' + (id+i)%26))
				s.Set(k, NewString("v"))
				_, _ = s.Get(k)
			}
		}(w)
	}
	wg.Wait()
}
