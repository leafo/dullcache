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
	availablePaths map[string]http.Header
	purgedMutex    sync.RWMutex
	purgedPaths    map[string]bool
}

func NewFileCache(basePath string) *FileCache {
	return &FileCache{
		basePath:       basePath,
		busyPaths:      make(map[string]bool),
		availablePaths: make(map[string]http.Header),
		purgedPaths:    make(map[string]bool),
	}
}

func (cache *FileCache) CountAvailablePaths() int {
	cache.availableMutex.RLock()
	defer cache.availableMutex.RUnlock()
	return len(cache.availablePaths)
}

func (cache *FileCache) CountBusyPaths() int {
	cache.busyMutex.RLock()
	defer cache.busyMutex.RUnlock()
	return len(cache.busyPaths)
}

func (cache *FileCache) CacheFilePath(subPath string) string {
	return path.Join(cache.basePath, subPath)
}

func (cache *FileCache) PathAvailable(path string) http.Header {
	cache.availableMutex.RLock()
	defer cache.availableMutex.RUnlock()
	return cache.availablePaths[path]
}

func (cache *FileCache) PathMaybeAvailable(path string) (int64, error) {
	info, err := os.Stat(cache.CacheFilePath(path))

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

func (cache *FileCache) MarkPathAvailable(path string, headers http.Header) {
	cache.availableMutex.Lock()
	defer cache.availableMutex.Unlock()
	cache.availablePaths[path] = headers
}

func (cache *FileCache) MarkPathBusy(path string) bool {
	cache.busyMutex.Lock()
	defer cache.busyMutex.Unlock()

	if cache.busyPaths[path] {
		return false
	}

	cache.busyPaths[path] = true
	return true
}

func (cache *FileCache) MarkPathFree(path string) {
	cache.busyMutex.Lock()
	defer cache.busyMutex.Unlock()
	delete(cache.busyPaths, path)
}

func (cache *FileCache) PathNeedsPurge(path string) bool {
	cache.purgedMutex.RLock()
	defer cache.purgedMutex.RUnlock()
	return cache.purgedPaths[path]
}

func (cache *FileCache) ReleasePathPurge(path string) bool {
	cache.purgedMutex.RLock()
	needsPurge := cache.purgedPaths[path]
	cache.purgedMutex.RUnlock()

	if needsPurge {
		cache.purgedMutex.Lock()
		defer cache.purgedMutex.Unlock()
		if cache.purgedPaths[path] {
			delete(cache.purgedPaths, path)
			return true
		}
	}

	return false
}

func (cache *FileCache) MarkPathNeedsPurge(path string) {
	cache.purgedMutex.Lock()
	defer cache.purgedMutex.Unlock()
	cache.purgedPaths[path] = true
}
