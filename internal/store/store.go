package store

import (
	"math/rand/v2"
	"sync"
	"time"
)

type EvictionPolicy string

const (
	NoEviction    EvictionPolicy = "noeviction"
	AllKeysRandom EvictionPolicy = "allkeys-random"
	VolatileTTL   EvictionPolicy = "volatile-ttl"
)

type Stats struct {
	Keys        int
	MemoryBytes int64
	Hits        int64
	Misses      int64
	ExpiredKeys int64
	EvictedKeys int64
	HashKeys    int
	ListKeys    int
	StringKeys  int
	Changes     int64
}

type UpdateFn func(obj *Object, present bool) (mutated, deleted bool)

type ViewFn func(obj Object, present bool)

type Store interface {
	Get(key string) (Object, bool)
	Set(key string, obj Object)
	Delete(key string) bool
	Exists(key string) bool
	SetExpiry(key string, expiresAtMs int64) bool
	Persist(key string) bool
	View(key string, fn ViewFn)
	Update(key string, fn UpdateFn)
	Sweep(nowMs int64, maxSample int) int
	Len() int
	Stats() Stats
	Each(fn func(key string, obj Object))
	FlushAll()
}

type MapStore struct {
	mu       sync.RWMutex
	m        map[string]Object
	memBytes int64
	hits     int64
	misses   int64
	expired  int64
	evicted  int64
	changes  int64

	maxKeys int
	policy  EvictionPolicy
}

func NewMap() *MapStore {
	return &MapStore{m: make(map[string]Object), policy: NoEviction}
}

func (s *MapStore) SetCapacity(maxKeys int, policy EvictionPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxKeys = maxKeys
	if policy == "" {
		policy = NoEviction
	}
	s.policy = policy
}

func (s *MapStore) Get(key string) (Object, bool) {
	s.mu.RLock()
	obj, ok := s.m[key]
	s.mu.RUnlock()
	if !ok {
		s.mu.Lock()
		s.misses++
		s.mu.Unlock()
		return Object{}, false
	}
	if obj.ExpiresAt == 0 || obj.ExpiresAt > time.Now().UnixMilli() {
		s.mu.Lock()
		s.hits++
		s.mu.Unlock()
		return obj, true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, stillThere := s.m[key]
	if stillThere && cur.ExpiresAt > 0 && cur.ExpiresAt <= time.Now().UnixMilli() {
		s.memBytes -= int64(cur.approxSize())
		delete(s.m, key)
		s.expired++
	}
	s.misses++
	return Object{}, false
}

func (s *MapStore) Set(key string, obj Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setLocked(key, obj)
}

func (s *MapStore) setLocked(key string, obj Object) {
	if prev, ok := s.m[key]; ok {
		s.memBytes -= int64(prev.approxSize())
	}
	s.m[key] = obj
	s.memBytes += int64(obj.approxSize())
	s.changes++
	s.maybeEvictLocked()
}

func (s *MapStore) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.m[key]
	if !ok {
		return false
	}
	s.memBytes -= int64(prev.approxSize())
	delete(s.m, key)
	s.changes++
	return true
}

func (s *MapStore) Exists(key string) bool {
	s.mu.RLock()
	obj, ok := s.m[key]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	if obj.ExpiresAt > 0 && obj.ExpiresAt <= time.Now().UnixMilli() {
		return false
	}
	return true
}

func (s *MapStore) SetExpiry(key string, expiresAtMs int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	obj, ok := s.m[key]
	if !ok {
		return false
	}
	obj.ExpiresAt = expiresAtMs
	s.m[key] = obj
	s.changes++
	return true
}

func (s *MapStore) Persist(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	obj, ok := s.m[key]
	if !ok || obj.ExpiresAt == 0 {
		return false
	}
	obj.ExpiresAt = 0
	s.m[key] = obj
	s.changes++
	return true
}

func (s *MapStore) View(key string, fn ViewFn) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	obj, ok := s.m[key]
	if ok && obj.ExpiresAt > 0 && obj.ExpiresAt <= time.Now().UnixMilli() {
		fn(Object{}, false)
		return
	}
	fn(obj, ok)
}

func (s *MapStore) Update(key string, fn UpdateFn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, present := s.m[key]
	if present && cur.ExpiresAt > 0 && cur.ExpiresAt <= time.Now().UnixMilli() {
		s.memBytes -= int64(cur.approxSize())
		delete(s.m, key)
		s.expired++
		cur, present = Object{}, false
	}
	prevSize := 0
	if present {
		prevSize = cur.approxSize()
	}
	obj := cur
	mutated, deleted := fn(&obj, present)
	if deleted {
		if present {
			s.memBytes -= int64(prevSize)
			delete(s.m, key)
			s.changes++
		}
		return
	}
	if !mutated {
		return
	}
	s.m[key] = obj
	s.memBytes += int64(obj.approxSize()) - int64(prevSize)
	s.changes++
	s.maybeEvictLocked()
}

func (s *MapStore) Sweep(nowMs int64, maxSample int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	expired := 0
	i := 0
	for k, obj := range s.m {
		if i >= maxSample {
			break
		}
		i++
		if obj.ExpiresAt > 0 && obj.ExpiresAt <= nowMs {
			s.memBytes -= int64(obj.approxSize())
			delete(s.m, k)
			expired++
		}
	}
	s.expired += int64(expired)
	return expired
}

func (s *MapStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

func (s *MapStore) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Stats{
		Keys:        len(s.m),
		MemoryBytes: s.memBytes,
		Hits:        s.hits,
		Misses:      s.misses,
		ExpiredKeys: s.expired,
		EvictedKeys: s.evicted,
		Changes:     s.changes,
	}
	for _, obj := range s.m {
		switch obj.Type {
		case TypeString:
			st.StringKeys++
		case TypeHash:
			st.HashKeys++
		case TypeList:
			st.ListKeys++
		}
	}
	return st
}

func (s *MapStore) Each(fn func(key string, obj Object)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UnixMilli()
	for k, obj := range s.m {
		if obj.ExpiresAt > 0 && obj.ExpiresAt <= now {
			continue
		}
		fn(k, obj)
	}
}

func (s *MapStore) FlushAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = make(map[string]Object)
	s.memBytes = 0
	s.changes++
}

func (s *MapStore) maybeEvictLocked() {
	if s.maxKeys <= 0 || s.policy == NoEviction {
		return
	}
	for len(s.m) > s.maxKeys {
		victim := s.pickVictimLocked()
		if victim == "" {
			return
		}
		if obj, ok := s.m[victim]; ok {
			s.memBytes -= int64(obj.approxSize())
			delete(s.m, victim)
			s.evicted++
		}
	}
}

func (s *MapStore) pickVictimLocked() string {
	const sample = 5
	switch s.policy {
	case AllKeysRandom:
		for k := range s.m {
			return k
		}
	case VolatileTTL:
		var victim string
		var soonest int64
		i := 0
		for k, obj := range s.m {
			if i >= sample {
				break
			}
			i++
			if obj.ExpiresAt == 0 {
				continue
			}
			if victim == "" || obj.ExpiresAt < soonest {
				victim, soonest = k, obj.ExpiresAt
			}
		}
		if victim != "" {
			return victim
		}
		for k := range s.m {
			return k
		}
	}
	return ""
}

var _ = rand.IntN
