package query

import (
	"container/list"
	"sync"
)

type cacheItem struct {
	key   interface{}
	value interface{}
}

type LRUCache struct {
	capacity int
	cache    map[interface{}]*list.Element
	list     *list.List
	mu       sync.Mutex
}

func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[interface{}]*list.Element),
		list:     list.New(),
	}
}

func (c *LRUCache) Get(key interface{}) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.list.MoveToFront(elem)
		// Return the actual value from cacheItem
		return elem.Value.(*cacheItem).value, true
	}
	return nil, false
}

func (c *LRUCache) Put(key, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.list.MoveToFront(elem)
		// Update the value in cacheItem
		elem.Value.(*cacheItem).value = value
		return
	}

	if c.list.Len() >= c.capacity {
		// Evict least recently used
		elem := c.list.Back()
		if elem != nil {
			delete(c.cache, elem.Value.(*cacheItem).key)
			c.list.Remove(elem)
		}
	}

	// Store as cacheItem containing the actual value
	elem := c.list.PushFront(&cacheItem{key, value})
	c.cache[key] = elem
}

func (c *LRUCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.list.Len()
}
