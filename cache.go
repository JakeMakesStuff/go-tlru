package tlru

import (
	"container/list"
	"runtime"
	"sync"
	"time"
)

// Defines an item in the cache.
type cacheItem struct {
	item      interface{}
	size      int
	destroyer *time.Timer
	element   *list.Element
}

// Defines the cache and all required items.
type Cache struct {
	// Defines the attributes for the base cache.
	m        sync.Mutex
	keyList  *list.List
	valueMap map[interface{}]*cacheItem
	maxLen   int
	maxBytes int

	// The total size of items in the cache.
	totalBytes int

	// Define the duration of an item in the cache.
	duration time.Duration
}

// Purge the first added item possible from the cache.
// THIS FUNCTION IS NOT THREAD SAFE FOR PERFORMANCE REASONS (DOUBLE LOCKING)! BE CAREFUL!
func (c *Cache) purgeFirst() {
	f := c.keyList.Front()
	if f == nil {
		return
	}
	c.keyList.Remove(f)
	item := c.valueMap[f.Value]
	c.totalBytes -= item.size
	item.destroyer.Stop()
	delete(c.valueMap, f.Value)
}

// Get is used to try and get a interface from the cache.
// The second boolean is meant to represent ok. If it's false, it was not in the cache.
func (c *Cache) Get(Key interface{}) (item interface{}, ok bool) {
	// Lock the mutex.
	c.m.Lock()

	// Try to get from the cache.
	x, ok := c.valueMap[Key]

	// If this isn't ok, we return here.
	if !ok {
		c.m.Unlock()
		return nil, false
	}

	// Revive the key.
	c.keyList.Remove(x.element)
	x.element = c.keyList.PushBack(Key)
	x.destroyer.Reset(c.duration)

	// Unlock the mutex.
	c.m.Unlock()

	// Return the item.
	return x.item, true
}

// Used to generate a destruction function for a item.
func (c *Cache) destroyItem(Key interface{}, timer bool) func() {
	return func() {
		// Lock the mutex.
		c.m.Lock()

		// Delete the item from the cache.
		item := c.valueMap[Key]
		c.keyList.Remove(item.element)
		delete(c.valueMap, Key)
		if !timer {
			item.destroyer.Stop()
		}
		c.totalBytes -= item.size

		// Unlock the mutex.
		c.m.Unlock()
	}
}

// Delete is used to delete an option from the cache.
func (c *Cache) Delete(Key interface{}) {
	c.destroyItem(Key, false)()
}

// Erase is used to erase the cache.
func (c *Cache) Erase() {
	c.m.Lock()
	c.keyList = list.New()
	for _, v := range c.valueMap {
		v.destroyer.Stop()
	}
	c.valueMap = map[interface{}]*cacheItem{}
	c.m.Unlock()
	c.totalBytes = 0
	runtime.GC()
}

// Set is used to set a key/value interface in the cache.
func (c *Cache) Set(Key, Value interface{}) {
	// Lock the mutex.
	c.m.Lock()

	// Check if the key already exists and the length.
	item, exists := c.valueMap[Key]

	// Get the total size.
	var total int
	if c.maxBytes != 0 {
		total = int(sizeof(Value))
		if total > c.maxBytes {
			// Don't cache this.
			c.m.Unlock()
			return
		}
	}

	// If the key already exists, we should revive the key. If not, we should push it and set a timer in the map.
	if exists {
		c.keyList.Remove(item.element)
		item.element = c.keyList.PushBack(Key)
		item.destroyer.Reset(c.duration)
		item.item = Value
	} else {
		// If the length is the max length, remove one.
		l := len(c.valueMap)
		if l == c.maxLen && c.maxLen != 0 {
			c.purgeFirst()
		}

		// Set the cache item.
		el := c.keyList.PushBack(Key)
		item = &cacheItem{
			item:      Value,
			destroyer: time.AfterFunc(c.duration, c.destroyItem(Key, true)),
			size:      total,
			element:   el,
		}
		c.valueMap[Key] = item
		c.totalBytes += total
	}

	// Ensure max bytes is greater than or equal to total bytes.
	for c.totalBytes > c.maxBytes {
		// Purge the first item.
		c.purgeFirst()
	}

	// Unlock the mutex.
	c.m.Unlock()
}

// NewCache is used to create the cache.
// Setting MaxLength of MaxBytes to 0 will mean unlimited.
func NewCache(MaxLength, MaxBytes int, Duration time.Duration) *Cache {
	return &Cache{
		keyList:  list.New(),
		valueMap: map[interface{}]*cacheItem{},
		maxLen:   MaxLength,
		maxBytes: MaxBytes,
		duration: Duration,
	}
}
