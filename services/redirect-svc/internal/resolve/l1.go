package resolve

import (
	"container/list"
	"sync"
	"time"
)

// l1 is a small in-process LRU with per-entry TTL in front of Redis.
//
// Links are immutable, so serving a slightly stale entry is always correct;
// the TTL only bounds memory churn and how long a deleted link (future
// feature) would keep redirecting. Its job is resilience: hot slugs keep
// resolving in ~100ns even while Redis or links-svc are slow or down
// (see docs/RESULTS.md, v0.1.0 vs v0.2.0).
type l1 struct {
	mu    sync.Mutex
	max   int
	ll    *list.List // front = most recently used
	items map[string]*list.Element
	now   func() time.Time
}

type l1Entry struct {
	key     string
	val     string
	expires time.Time
}

func newL1(size int) *l1 {
	return &l1{max: size, ll: list.New(), items: make(map[string]*list.Element, size), now: time.Now}
}

func (c *l1) get(key string) (string, bool) {
	if c == nil || c.max <= 0 {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	e := el.Value.(*l1Entry)
	if c.now().After(e.expires) {
		c.ll.Remove(el)
		delete(c.items, key)
		return "", false
	}
	c.ll.MoveToFront(el)
	return e.val, true
}

func (c *l1) set(key, val string, ttl time.Duration) {
	if c == nil || c.max <= 0 || ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	exp := c.now().Add(ttl)
	if el, ok := c.items[key]; ok {
		e := el.Value.(*l1Entry)
		e.val, e.expires = val, exp
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(&l1Entry{key: key, val: val, expires: exp})
	for c.ll.Len() > c.max {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.items, last.Value.(*l1Entry).key)
	}
}

func (c *l1) len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
