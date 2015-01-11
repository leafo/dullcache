package dullcache

import (
	"sync"
	"sync/atomic"
)

type serverStats struct {
	bytesFetched    uint64
	bytesSent       uint64
	fastHits        uint64
	checkedHits     uint64
	passes          uint64
	stores          uint64
	activeTransfers int64
	sizeDist        map[uint64]uint64

	sync.RWMutex
}

var sizeDistsMB = []uint64{0, 1, 10, 20, 30, 50, 100, 200, 500, 750}

func newServerStats() *serverStats {
	return &serverStats{
		sizeDist: make(map[uint64]uint64),
	}
}

// amount in bytes
func (stats *serverStats) incrSizeDist(amount uint64) {
	mb := amount / (1024 * 1024)
	for i := len(sizeDistsMB) - 1; i >= 0; i-- {
		if mb >= sizeDistsMB[i] {
			stats.Lock()
			defer stats.Unlock()
			stats.sizeDist[sizeDistsMB[i]] += 1
			return
		}
	}
}

func (stats *serverStats) incrBytesFetched(amount uint64) {
	atomic.AddUint64(&stats.bytesFetched, amount)
}

func (stats *serverStats) incrBytesSent(amount uint64) {
	atomic.AddUint64(&stats.bytesSent, amount)
}

func (stats *serverStats) incrFastHits(amount uint64) {
	atomic.AddUint64(&stats.fastHits, amount)
}

func (stats *serverStats) incrCheckedHits(amount uint64) {
	atomic.AddUint64(&stats.checkedHits, amount)
}

func (stats *serverStats) incrPasses(amount uint64) {
	atomic.AddUint64(&stats.passes, amount)
}

func (stats *serverStats) incrStores(amount uint64) {
	atomic.AddUint64(&stats.stores, amount)
}

func (stats *serverStats) incrActiveTransfers(amount int64) {
	atomic.AddInt64(&stats.activeTransfers, amount)
}
