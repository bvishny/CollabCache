package main

/* BEGIN PRIORITY QUEUE
   Source: http://golang.org/pkg/container/heap/
 */

import (
	"sync"
	"container/heap"
)

type PriorityQueue struct {
	items *PriorityList
	sync.RWMutex
}

type PriorityList []*PriorityItem

func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{
		items: &PriorityList{},
	}
	
	heap.Init(pq.items)
	return pq
}

type PriorityItem struct {
	value interface{}
	priority int
}

func (pl PriorityList) Len() int           { return len(pl) }
func (pl PriorityList) Less(i, j int) bool { return pl[i].priority < pl[j].priority }
func (pl PriorityList) Swap(i, j int)      { pl[i], pl[j] = pl[j], pl[i] }

func (pq *PriorityQueue) Push(x interface{}, priority int) {
	pq.Lock()
	defer pq.Unlock()
	
	heap.Push(pq.items, &PriorityItem{x, priority})
}

func (pl *PriorityList) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*pl = append(*pl, x.(*PriorityItem))
}

func (pq *PriorityQueue) Pop() *PriorityItem {
	pq.Lock()
	defer pq.Unlock()
	
	return heap.Pop(pq.items).(*PriorityItem)
}

func (pq *PriorityQueue) Len() int {
	pq.RLock()
	defer pq.RUnlock()
	
	return pq.items.Len()
}

func (pq *PriorityQueue) Peek() *PriorityItem {
	pq.RLock()
	defer pq.RUnlock()
	
	if pq.items.Len() == 0 {
		return nil
	}
	
	return (*pq.items)[0]
}

func (pl *PriorityList) Pop() interface{} {
	old := *pl
	n := len(old)
	x := old[n-1]
	*pl = old[0 : n-1]
	return x
}

/* END PRIORITY QUEUE */