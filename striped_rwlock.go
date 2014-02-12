package main

import (
	"sync"
)

const (
	DefaultLockBuckets = 8
)

// this is the java.lang.String.hashCode() - taken from https://github.com/bjarneh/bloomfilter/blob/master/bloomfilter.go
var JavaHashFunc func(string) uint32 = func(s string) uint32 {
    var val uint32 = 1
    for i := 0; i < len(s); i++ {
        val += (val * 37) + uint32(s[i])
    }
    return val
}

type StripedRWLock struct {
	hashFunc func(string) uint32
	numBuckets uint32
	buckets []sync.RWMutex
}

func NewStripedRWLock(numBuckets uint32, hashFunc func(string) uint32) *StripedRWLock {
	// Use java hashCode if none specified
	if hashFunc == nil {
		hashFunc = JavaHashFunc
	}
	
	return &StripedRWLock{
		hashFunc: hashFunc,
		numBuckets: numBuckets,
		buckets: make([]sync.RWMutex, numBuckets),
	}
}

func (self *StripedRWLock) Lock(key string) {
	self.lockFor(key).Lock()
}

func (self *StripedRWLock) Unlock(key string) {
	self.lockFor(key).Unlock()
}

func (self *StripedRWLock) RLock(key string) {
	self.lockFor(key).RLock()
}

func (self *StripedRWLock) RUnlock(key string) {
	self.lockFor(key).RUnlock()
}

// private api..
func (self *StripedRWLock) lockFor(key string) *sync.RWMutex {
	return &self.buckets[self.hashFunc(key) % self.numBuckets]
}