package query

import (
	"container/list"
	"sync"
	"time"
)

type cacheItem struct {
	key       interface{}
	value     interface{}
	expiresAt time.Time
}

type LRUCache struct {
	capacity int
	ttl      time.Duration
	cache    map[interface{}]*list.Element
	list     *list.List
	mu       sync.RWMutex
}

func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	c := &LRUCache{
		capacity: capacity,
		ttl:      ttl,
		cache:    make(map[interface{}]*list.Element),
		list:     list.New(),
	}

	go c.cleanup()
	return c
}

func (c *LRUCache) Get(key interface{}) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		item := elem.Value.(*cacheItem)
		if c.ttl > 0 && time.Now().After(item.expiresAt) {
			c.removeElement(elem)
			return nil, false
		}

		c.list.MoveToFront(elem)
		return item.value, true
	}
	return nil, false
}

func (c *LRUCache) Put(key, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiresAt := now.Add(c.ttl)
	if c.ttl == 0 {
		expiresAt = time.Time{}
	}

	if elem, ok := c.cache[key]; ok {
		c.list.MoveToFront(elem)
		item := elem.Value.(*cacheItem)
		item.value = value
		item.expiresAt = expiresAt
		return
	}

	if c.list.Len() >= c.capacity {
		elem := c.list.Back()
		if elem != nil {
			c.removeElement(elem)
		}
	}

	item := &cacheItem{key, value, expiresAt}
	elem := c.list.PushFront(item)
	c.cache[key] = elem
}

func (c *LRUCache) removeElement(elem *list.Element) {
	delete(c.cache, elem.Value.(*cacheItem).key)
	c.list.Remove(elem)
}

func (c *LRUCache) cleanup() {
	if c.ttl == 0 {
		return
	}

	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		var toRemove []*list.Element

		for elem := c.list.Back(); elem != nil; elem = elem.Prev() {
			item := elem.Value.(*cacheItem)
			if now.After(item.expiresAt) {
				toRemove = append(toRemove, elem)
			} else {
				break
			}
		}

		for _, elem := range toRemove {
			c.removeElement(elem)
		}
		c.mu.Unlock()
	}
}

func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.list.Len()
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[interface{}]*list.Element)
	c.list.Init()
}
