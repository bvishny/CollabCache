CollabCache
===========

CollabCache is a concurrent key-value store designed to store values that are **expensive to generate**. CollabCache has a single call:

    **GetOrPublish(key string, ttl int64, generatorFunc func(string) interface{})**

which takes a key, a TTL in seconds, and an anonymous function to generate the corresponding value. CollabCache ensures that the anonymous function (an expensive operation) is only run once - by the first goroutine to call GetOrPublish. Other goroutines will block until the this call completes then return the generated value. 

CollabCache has a simple TTL system with both **stale** and **expired** keys. A key becomes stale after ttl/2 seconds. The next time it is requested (before expiration), a goroutine will be spun off to regenerate the value. This operation is non-blocking, and other goroutines can enjoy fast access to the stale value during this time. Once keys expire, they are physically deleted from the cache by a goroutine dedicated to deletion. 

Usage
===========
    import "time"
  
    // Sending a boolean over or closing quitChan will kill the deletion goroutine
    quitChan := make(chan bool)
    // Create a new Collab Cache
    cache := NewCollabCache(quitChan)
    // Example anonymous generator function
    generator := func(key string) interface{} { 
      time.Sleep(time.Duration(20) * time.Millisecond)
      return "hello"
    }
    // GetOrPublish key hello with ttl 60 seconds and generation function generator
    cache.GetOrPublish("hello", 60, generator)
  
