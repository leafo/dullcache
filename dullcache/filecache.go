package dullcache

import (
	"net/http"
	"os"
	"path"
	"sync"
)

type FileCache struct {
	basePath       string
	busyMutex      sync.RWMutex
	busyPaths      map[string]bool
	availableMutex sync.RWMutex
	availablePaths map[string]*http.Header
}

func NewFileCache(basePath string) *FileCache {
	return &FileCache{
		basePath:       basePath,
		busyPaths:      make(map[string]bool),
		availablePaths: make(map[string]*http.Header),
	}
}

func (cache *FileCache) CountPathsCached() int {
	cache.availableMutex.RLock()
	defer cache.availableMutex.RUnlock()
	return len(cache.availablePaths)
}

func (cache *FileCache) fullCachePath(subPath string) string {
	return path.Join(cache.basePath, subPath)
}

func (cache *FileCache) PathAvailable(path string) *http.Header {
	cache.availableMutex.RLock()
	defer cache.availableMutex.RUnlock()
	return cache.availablePaths[path]
}

func (cache *FileCache) PathMaybeAvailable(path string) (int64, error) {
	info, err := os.Stat(cache.fullCachePath(path))

	if os.IsNotExist(err) {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}

	return info.Size(), nil
}

func (cache *FileCache) PathBusy(path string) bool {
	cache.busyMutex.RLock()
	defer cache.busyMutex.RUnlock()
	return cache.busyPaths[path]
}

func (cache *FileCache) MarkPathAvailable(path string, headers *http.Header) {
	cache.availableMutex.Lock()
	defer cache.availableMutex.Unlock()
	cache.availablePaths[path] = headers
}
