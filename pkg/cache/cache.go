package cache

import (
	"errors"
	"sync"
	"time"
)

var ErrInvalidDuration = errors.New("expireAfter duration cannot be negative")

const initialSize = 16

type Func[T any] func() (T, error)

type Cache[T any] struct {
	data        map[string]item[T]
	expireAfter time.Duration
	mu          sync.RWMutex
	stopChan    chan bool
	ticker      *time.Ticker
}

type item[T any] struct {
	object    T
	expiresAt int64
}

func (c *item[T]) expired() bool {
	return c.expiresAt < time.Now().UnixNano()
}

// New creates a new in memory cache.
func New[T any](expireAfter time.Duration) (*Cache[T], error) {
	if expireAfter < 0 {
		return nil, ErrInvalidDuration
	}

	markInterval := expireAfter / 2
	if markInterval <= 0 {
		markInterval = time.Second
	}

	c := &Cache[T]{
		data:        make(map[string]item[T], initialSize),
		expireAfter: expireAfter,
		stopChan:    make(chan bool),
		ticker:      time.NewTicker(markInterval),
	}

	go c.backgroundExpire()

	return c, nil
}

func (c *Cache[T]) backgroundExpire() {
	for {
		select {
		case <-c.stopChan:
			close(c.stopChan)
			return
		case t := <-c.ticker.C:
			c.mark(t.UnixNano())
		}
	}
}

// mark takes a read lock to search for expired entries and
// enqueues deletions in separate background functions.
func (c *Cache[T]) mark(t int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for k, v := range c.data {
		k := k
		if t > v.expiresAt {
			go c.purgeExpired(k, v.expiresAt)
		}
	}
}

// Stop will shutdown the background cleanup for the cache.
func (c *Cache[T]) Stop() {
	c.ticker.Stop()
	c.stopChan <- true
}

// Removes an item by name and expiry time when the purge was scheduled.
// If there is a race, and the item has been refreshed, it will not be purged.
func (c *Cache[T]) purgeExpired(name string, expectedExpiryTime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, ok := c.data[name]; ok && item.expiresAt == expectedExpiryTime {
		// found, and the expiry time is still the same as when the purge was requested.
		delete(c.data, name)
	}
}

// Size returns the number of items in the cache.
func (c *Cache[T]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// Clear removes all items from the cache, regardless of their expiration.
func (c *Cache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]item[T], initialSize)
}

// WriteThruLookup checks the cache for the value associated with name,
// and if not found or expired, invokes the provided primaryLookup function
// to local the value.
func (c *Cache[T]) WriteThruLookup(name string, primaryLookup Func[T]) (T, error) {
	var nilT T

	c.mu.RLock()
	val, hit := c.lookup(name)
	if hit {
		c.mu.RUnlock()
		return val, nil
	}
	c.mu.RUnlock()

	// Ensure the value hasn't been set by another goroutine by escalating to a RW
	// lock. We need the W lock anyway if we're about to write.
	c.mu.Lock()
	defer c.mu.Unlock()
	val, hit = c.lookup(name)
	if hit {
		return val, nil
	}

	// If we got this far, it was either a miss, or hit w/ expired value, execute
	// the function.

	// Value does indeed need to be refreshed. Used the provided function.
	newData, err := primaryLookup()
	if err != nil {
		return nilT, err
	}

	// save the newData in the cache. newData may be nil, if that's what the WriteThruFunction provided.
	c.data[name] = item[T]{
		object:    newData,
		expiresAt: time.Now().Add(c.expireAfter).UnixNano(),
	}
	return newData, nil
}

// Lookup checks the cache for a non-expired object by the supplied key name.
// The bool return informs the caller if there was a cache hit or not.
// A return of nil, true means that nil is in the cache.
// Where nil, false indicates a cache miss or that the value is expired and should
// be refreshed.
func (c *Cache[T]) Lookup(name string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lookup(name)
}

// Set saves the current value of an object in the cache, with the supplied
// durintion until the object expires.
func (c *Cache[T]) Set(name string, object T) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[name] = item[T]{
		object:    object,
		expiresAt: time.Now().Add(c.expireAfter).UnixNano(),
	}

	return nil
}

// lookup finds an unexpired item at the given name. The bool indicates if a hit
// occurred. This is an internal API that is NOT thread-safe. Consumers must
// take out a read or read-write lock.
func (c *Cache[T]) lookup(name string) (T, bool) {
	var nilT T
	if item, ok := c.data[name]; ok && item.expired() {
		// Cache hit, but expired. The removal from the cache is deferred.
		go c.purgeExpired(name, item.expiresAt)
		return nilT, false
	} else if ok {
		// Cache hit, not expired.
		return item.object, true
	}

	// Cache miss.
	return nilT, false
}
