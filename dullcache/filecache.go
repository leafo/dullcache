package dullcache

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync"
	"syscall"

	"github.com/stvp/slug"
)

type FileCache struct {
	basePath       string
	busyMutex      sync.RWMutex
	busyPaths      map[string]bool
	availableMutex sync.RWMutex
	availablePaths map[string]http.Header
	purgedMutex    sync.RWMutex
	purgedPaths    map[string]bool
	accessList     *AccessList
}

func NewFileCache(basePath string) *FileCache {
	return &FileCache{
		basePath:       basePath,
		accessList:     NewAccessList(),
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

func (cache *FileCache) CountPurgedPaths() int {
	cache.purgedMutex.RLock()
	defer cache.purgedMutex.RUnlock()
	return len(cache.purgedPaths)
}

func (cache *FileCache) CacheFilePath(subPath string) (string, error) {
	slug := slug.Clean(subPath)

	if slug == "" {
		return "", fmt.Errorf("invalid slug")
	}

	return path.Join(cache.basePath, slug), nil
}

func (cache *FileCache) PathAvailable(path string) http.Header {
	cache.availableMutex.RLock()
	defer cache.availableMutex.RUnlock()
	return cache.availablePaths[path]
}

func (cache *FileCache) PathMaybeAvailable(path string) (int64, error) {
	path, err := cache.CacheFilePath(path)

	if err != nil {
		return 0, err
	}

	info, err := os.Stat(path)

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

// size in bytes of all the available paths
func (cache *FileCache) TrackedSize() int64 {
	cache.availableMutex.RLock()
	defer cache.availableMutex.RUnlock()

	var total int64

	for _, headers := range cache.availablePaths {
		contentLenStr := headers.Get("Content-Length")
		if contentLenStr != "" {
			contentLen, err := strconv.Atoi(contentLenStr)
			if err == nil {
				total += int64(contentLen)
			}

		}
	}

	return total
}

func (cache *FileCache) DeletePath(path string) error {
	fname, err := fileCache.CacheFilePath(path)

	if err != nil {
		return err
	}

	if !cache.MarkPathBusy(path) {
		return fmt.Errorf("path is busy")
	}

	// remove from everything
	cache.availableMutex.Lock()
	defer cache.availableMutex.Unlock()
	delete(cache.availablePaths, path)

	cache.purgedMutex.Lock()
	defer cache.purgedMutex.Unlock()
	delete(cache.purgedPaths, path)

	cache.accessList.RemovePath(path)

	cache.busyMutex.Lock()
	defer cache.busyMutex.Unlock()
	delete(cache.busyPaths, path)

	return syscall.Unlink(fname)
}
