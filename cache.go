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
	size	  int
	destroyer *time.Timer
}

// Defines a key in the key list.
type cacheKey struct {
	key  interface{}
	item *cacheItem
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
	c.totalBytes -= f.Value.(*cacheKey).item.size
	f.Value.(*cacheKey).item.destroyer.Stop()
	delete(c.valueMap, f.Value.(*cacheKey).key)
}

// Revives a key. This will reset its expiry and push it to the back of the list.
// We do this from the back of the list since if it's just being fetched, it's more likely to be new.
// THIS FUNCTION IS NOT THREAD SAFE FOR PERFORMANCE REASONS (DOUBLE LOCKING)! BE CAREFUL!
func (c *Cache) revive(key interface{}) {
	e := c.keyList.Back()
	push := func() {
		value := c.valueMap[key]
		c.keyList.PushBack(&cacheKey{
			key:  key,
			item: value,
		})
		value.destroyer.Reset(c.duration)
	}
	for e != nil {
		k := e.Value.(*cacheKey)
		if k.key == key {
			c.keyList.Remove(e)
			push()
			return
		}
		e = e.Prev()
	}
	push()
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
	c.revive(Key)

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
		delete(c.valueMap, Key)
		x := c.keyList.Back()
		for x != nil {
			ck := x.Value.(*cacheKey)
			if ck.key == Key {
				c.keyList.Remove(x)
				if !timer {
					ck.item.destroyer.Stop()
				}
				c.totalBytes -= ck.item.size
				break
			}
			x.Prev()
		}

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
		c.revive(Key)
		item.item = Value
	} else {
		// If the length is the max length, remove one.
		l := len(c.valueMap)
		if l == c.maxLen && c.maxLen != 0 {
			c.purgeFirst()
		}

		// Set the cache item.
		item = &cacheItem{
			item:      Value,
			destroyer: time.AfterFunc(c.duration, c.destroyItem(Key, true)),
			size: total,
		}
		c.keyList.PushBack(&cacheKey{
			key:  Key,
			item: item,
		})
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
