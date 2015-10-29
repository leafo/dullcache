package dullcache

import (
	"sync"
	"time"

	"github.com/ryszard/goskiplist/skiplist"
)

type AccessList struct {
	ordered   *skiplist.Set
	pathTimes map[string]int64
	mutex     sync.RWMutex
}

func NewAccessList() *AccessList {
	var list *AccessList
	list = &AccessList{
		ordered: skiplist.NewCustomSet(func(l, r interface{}) bool {
			lstring := l.(string)
			rstring := r.(string)

			ltime := list.pathTimes[lstring]
			rtime := list.pathTimes[rstring]

			if ltime == rtime {
				return lstring < rstring
			}

			return ltime < rtime
		}),
		pathTimes: make(map[string]int64),
	}

	return list
}

func (list *AccessList) AccessPath(path string) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	// remove first so we can re-order correctly with new time
	list.ordered.Remove(path)
	list.pathTimes[path] = time.Now().Unix()
	list.ordered.Add(path)
}

func (list *AccessList) RemovePath(path string) {
	list.mutex.Lock()
	defer list.mutex.Unlock()

	list.ordered.Remove(path)
	delete(list.pathTimes, path)
}
