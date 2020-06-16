package tlru

import (
	"container/list"
	"sync"
	"time"
)

// Defines the cache and all required items.
type Cache struct {
	// Defines the attributes for the base cache.
	m        *sync.RWMutex
	keyList  *list.List
	valueMap map[interface{}]interface{}
	maxLen   int

	// Define the duration of an item in the cache.
	duration time.Duration

	// Handles ensuring the expiry loop will actually die but can be resurrected.
	loopKilled     bool
	loopKilledLock *sync.Mutex
}

// Defines a key in the cache.
type cacheKey struct {
	key       interface{}
	expiresAt time.Time
}

// Defines the loop used to purge expired items from the cache.
func (c *Cache) purgeExpiredLoop() {
	// Wait a millisecond before starting so that we are definitely behind times but not by much (prevents wastage).
	time.Sleep(time.Millisecond)

	for {
		// Sleep for the specified duration.
		time.Sleep(c.duration)

		// Get the time right now.
		t := time.Now().UTC()

		// Lock the cache. This isn't a read lock since there is a high chance of eviction (or returning quickly).
		c.m.Lock()

		// Get the first element.
		e := c.keyList.Front()

		// Is the first element nil?
		if e == nil {
			// It is. Ok, this cache is empty so this loop shouldn't be running. Unlock and return.
			c.m.Unlock()
			return
		}

		// Loop through the cache.
		for e != nil {
			// Get the cache item.
			item := e.Value.(cacheKey)

			if !item.expiresAt.Before(t) {
				// Ok. We do not need to purge anything further, we can break now.
				break
			}

			// Remove from the list.
			c.keyList.Remove(e)

			// Remove from the value map.
			delete(c.valueMap, item.key)

			// Get the next item in the cache.
			e = e.Next()
		}

		// Unlock the cache.
		c.m.Unlock()
	}
}

// Purge the first added item possible from the cache.
// THIS FUNCTION IS NOT THREAD SAFE FOR PERFORMANCE REASONS (DOUBLE LOCKING)! BE CAREFUL!
func (c *Cache) purgeFirst() {
	f := c.keyList.Front()
	if f == nil {
		return
	}
	c.keyList.Remove(f)
	delete(c.valueMap, f.Value.(cacheKey).key)
}

// Revives a key. This will reset its expiry and push it to the back of the list.
// We do this from the back of the list since if it's just being fetched, it's more likely to be new.
// THIS FUNCTION IS NOT THREAD SAFE FOR PERFORMANCE REASONS (DOUBLE LOCKING)! BE CAREFUL!
func (c *Cache) revive(key interface{}) {
	e := c.keyList.Back()
	push := func() {
		c.keyList.PushBack(cacheKey{
			key:       key,
			expiresAt: time.Now().UTC().Add(c.duration),
		})
	}
	for e != nil {
		k := e.Value.(cacheKey).key
		if k == key {
			c.keyList.Remove(e)
			push()
			return
		}
		e = e.Prev()
	}
	push()
}

// Ensures the loop is running. This is important if the cache has a lot of expired elements.
func (c *Cache) ensureLoop() {
	// Lock the loop kill lock. This isn't a RWMutex since that has the potential to add a race condition.
	c.loopKilledLock.Lock()

	// If it was killed, run it again.
	if c.loopKilled {
		c.loopKilled = false
		go c.purgeExpiredLoop()
	}

	// Re-unlock the boolean.
	c.loopKilledLock.Unlock()
}

// Get is used to try and get a interface from the cache.
// The second boolean is meant to represent ok. If it's false, it was not in the cache.
func (c *Cache) Get(Key interface{}) (item interface{}, ok bool) {
	// Try to get from the cache.
	c.m.RLock()
	x, ok := c.valueMap[Key]
	c.m.RUnlock()

	// If this isn't ok, we return here.
	if !ok {
		return nil, false
	}

	// Start a go-routine to revive the key in the cache and ensure the loop is running.
	go func() {
		c.ensureLoop()
		c.m.Lock()
		c.revive(Key)
		c.m.Unlock()
	}()

	// Return the item.
	return x, true
}

// Set is used to set a key/value interface in the cache.
func (c *Cache) Set(Key, Value interface{}) {
	// Ensure the loop is running.
	go c.ensureLoop()

	// Read lock the cache and check if the key already exists and the length.
	c.m.RLock()
	_, exists := c.valueMap[Key]
	l := len(c.valueMap)
	c.m.RUnlock()

	// Lock the mutex.
	c.m.Lock()

	// If the length is the max length, remove one.
	if l == c.maxLen {
		c.purgeFirst()
	}

	// If the key already exists, we should revive the key. If not, we should push it.
	if exists {
		c.revive(Key)
	} else {
		c.keyList.PushBack(cacheKey{
			key:       Key,
			expiresAt: time.Now().UTC().Add(c.duration),
		})
	}

	// Set the item.
	c.valueMap[Key] = Value

	// Unlock the mutex.
	c.m.Unlock()
}

// NewCache is used to create the cache.
func NewCache(MaxLength int, Duration time.Duration) *Cache {
	return &Cache{
		m:              &sync.RWMutex{},
		keyList:        list.New(),
		valueMap:       map[interface{}]interface{}{},
		maxLen:         MaxLength,
		duration:       Duration,
		loopKilled:     true,
		loopKilledLock: &sync.Mutex{},
	}
}
