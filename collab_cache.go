package main

/* BEGIN COLLAB CACHE */

import (
	"sync"
	"sync/atomic"
)

const (
	StateCreated = iota // Standard state
	StateRefreshing = iota // This key is being refreshed by a non-blocking operation
	StateDeleting = iota // This key is being deleted
)

/* A special type of concurrent cache that prevents duplicate work
   when publishing to a given cache key. 

   CAVEATS: 
   1) Not designed to store nil values
*/
type CollabCache struct {
	cache map[string]*CacheItem
	lock *StripedRWLock // Used for striped locking on most operations
	initLock *StripedRWLock // Used for striped locking specifically on key initialization
	pqueue *PriorityQueue // a thread-safe priority queue used to store expirations by key
	quitChannel chan bool // Receives global quit message and shuts down invalidator
}

// Create a new CollabCache passing in a quit channel for the Invalidator goroutine
func NewCollabCache(quitChannel chan bool) *CollabCache {
	cache := &CollabCache{
		cache: make(map[string]*CacheItem),
		lock: NewStripedRWLock(DefaultLockBuckets, JavaHashFunc),
		initLock: NewStripedRWLock(DefaultLockBuckets / 2, JavaHashFunc),
		pqueue: NewPriorityQueue(),
		quitChannel: quitChannel,
	}
	
	go cache.invalidator()
	return cache
}


/* A Cache Item - contains a sync.Once, an item (interface), and an expiration time */
type CacheItem struct {
	item interface{} // The stored value
	expires int64 // The expiration time in seconds
	stale int64 // The stale time in seconds
	state uint32 // Indicates if regeneration of stale val or deletion in progress
	sync.RWMutex
}

// Create a new CacheItem
func NewCacheItem() *CacheItem {
	return &CacheItem{
		item: nil,
		expires: 0,
		stale: 0,
		state: StateCreated,
	}
}

// Either gets a value or publish that value using generatorFunc if not available
// ttl is specified in seconds
func (self *CollabCache) GetOrPublish(key string, ttl int64, generatorFunc func(string) interface{}) interface{} {
	self.lock.RLock(key)
	defer self.lock.RUnlock(key)
	
	// initializeKey automatically locks item mutex
	// First thread to initialize item gets lock and calls generatorFunc
	if (self.cache[key] == nil || self.isExpired(key)) && self.initializeKey(key) {
		self.writeItem(key, ttl, generatorFunc(key))
		defer self.cache[key].Unlock()
	} else {
		self.cache[key].RLock()
		defer self.cache[key].RUnlock()
	}
	
	// First thread to call this on a stale key spawns a new goroutine that regens the value. Non-blocking op.
	self.refreshIfStale(key, ttl, generatorFunc)
	
	return self.cache[key].item
 }
  
 // A goroutine that invalidates items every second
func (self *CollabCache) invalidator() {
 	ticker := time.NewTicker(time.Duration(1) * time.Second)
	
	for {
		select {
		case time := <-ticker.C:
			{
				now := time.Unix()
				
				// Priority is the expiration time. PQueue is sorted by lowest expiration time
				// Theoretically item could be different than the popped value. But popped value must have even lower expiration.
				// Since this thread is currently the sole Popper we also don't risk popping an empty PQueue
				for item := self.pqueue.Peek(); item != nil && int64(item.priority) <= now; item = self.pqueue.Peek()  {
					self.invalidate(self.pqueue.Pop().value.(string)) // items are strings corresponding to key names
				}
			}
		case <- self.quitChannel: return
		}
	}
}

 /* Private API: 
    IMPORTANT: Not all private functions are thread-safe. Thread-safe methods are listed first. 
    Methods marked as NOT thread-safe must be called within an existing synchronization context 
    and used with caution.
 */

/* THE BELOW FUNCTIONS ARE THREAD-SAFE: */

 // Invalidate a given key value
func (self *CollabCache) invalidate(key string) {
	self.lock.Lock(key)
	defer self.lock.Unlock(key)

	self.cache[key].Lock()
	defer self.cache[key].Unlock()
	
	if self.isExpired(key) && self.testAndCAS(&self.cache[key].state, StateCreated, StateDeleting) {
		delete(self.cache, key)
	}
}

// Initialize a given key (either non-existent or expired) and return true if I am first thread to do so
func (self *CollabCache) initializeKey(key string) bool {
	self.initLock.Lock(key)
	defer self.initLock.Unlock(key)
	
	if self.cache[key] == nil || self.isExpired(key) {
		if self.cache[key] == nil {
			self.cache[key] = NewCacheItem()
		}
		self.cache[key].Lock()
		return true
	}
	
	return false
}

/* BELOW CODE IS NOT THREAD-SAFE - MUST BE CALLED WITHIN A THREAD-SAFE CONTEXT! */

/* Helper method that writes an item to a CacheItem and fills in expiration fields 
   Single point of registration for expiration pqueue
*/
func (self *CollabCache) writeItem(key string, ttl int64, item interface{}) {
	self.cache[key].item = item
	now := time.Now().Unix()
	
	// If ttl is 0 key will not expire or become stale
	if ttl != 0 {
		self.cache[key].expires = now + ttl
		self.cache[key].stale = now + (ttl / 2)
		
		// Add to priority queue with expire time as priority
		self.pqueue.Push(key, int(now + ttl))
	}
}

// Indicates whether a given key is expired
func (self *CollabCache) isExpired(key string) bool {
	return self.cache[key].expires != 0 && self.cache[key].expires <= time.Now().Unix()
}

// Indicates whether a given key is Stale
func (self *CollabCache) isStale(key string) bool {
	return self.cache[key].stale != 0 && self.cache[key].stale <= time.Now().Unix()
}

/* This function is a non-blocking call that regenerates a stale key - if the current thread is the first caller 
   The key in question cannot be deleted until the operation finishes
*/
func (self *CollabCache) refreshIfStale(key string, ttl int64, generatorFunc func(string) interface{}) {
	// Ensures that only the first caller regenerates the value, and that it will not be deleted while this is in process
	if self.isStale(key) && self.testAndCAS(&self.cache[key].state, StateCreated, StateRefreshing) {
		go func() {
			  item := generatorFunc(key)
			  // Wait until generation finishes to secure lock
			  self.cache[key].Lock()
			  defer self.cache[key].Unlock()
			  
			  self.writeItem(key, ttl, item)
			  atomic.StoreUint32(&self.cache[key].state, StateCreated)
		}()
	}
}

// Essentially Test-And-Test-And-Set with CompareAndSwap - tests the value before CompareAndSwap so as to reduce unnecessary contention
func (self *CollabCache) testAndCAS(ptr *uint32, old uint32, new uint32) bool {
	return atomic.LoadUint32(ptr) == old && atomic.CompareAndSwapUint32(ptr, old, new)
}
/* END COLLAB CACHE */
