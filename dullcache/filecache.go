package dullcache

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync"
	"syscall"

	b58 "github.com/jbenet/go-base58"
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

// Returns the total number of paths the file cache is tracking
func (cache *FileCache) CountAvailablePaths() int {
	cache.availableMutex.RLock()
	defer cache.availableMutex.RUnlock()
	return len(cache.availablePaths)
}

// Returns the total number of paths that are currently marked as busy
func (cache *FileCache) CountBusyPaths() int {
	cache.busyMutex.RLock()
	defer cache.busyMutex.RUnlock()
	return len(cache.busyPaths)
}

// Returns the total number of paths that are currently marked as purged
func (cache *FileCache) CountPurgedPaths() int {
	cache.purgedMutex.RLock()
	defer cache.purgedMutex.RUnlock()
	return len(cache.purgedPaths)
}

// Takes a subpath from the original request and converts it to a path on the
// filesystem where the cache should store it's copy of the file
func (cache *FileCache) CacheFilePath(subPath string) (string, error) {
	fname := b58.Encode([]byte(subPath))

	if fname == "" {
		return "", fmt.Errorf("failed to generate path for cache file")
	}

	return path.Join(cache.basePath, fname), nil
}

// Checks if a path is available for being served to client
func (cache *FileCache) PathAvailable(path string) http.Header {
	cache.availableMutex.RLock()
	defer cache.availableMutex.RUnlock()
	return cache.availablePaths[path]
}

// Check if we have the path on the filesystem but not tracked. This is used to
// keep re-use cache files across server restarts
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

// Checks if a path is currently busy
func (cache *FileCache) PathBusy(path string) bool {
	cache.busyMutex.RLock()
	defer cache.busyMutex.RUnlock()
	return cache.busyPaths[path]
}

// Mark a path as being available to be served by the cache, takes the headers
// from the backend request to fetch the file
func (cache *FileCache) MarkPathAvailable(path string, headers http.Header) {
	cache.availableMutex.Lock()
	defer cache.availableMutex.Unlock()
	cache.availablePaths[path] = headers
}

// Marks a path as busy. Paths should be marked busy when any write disk
// operation are happening so no new requests try to consume the file
//
// Returns true if it was able to mark the bath busy, false otherwise
func (cache *FileCache) MarkPathBusy(path string) bool {
	cache.busyMutex.Lock()
	defer cache.busyMutex.Unlock()

	if cache.busyPaths[path] {
		return false
	}

	cache.busyPaths[path] = true
	return true
}

// Mark a path as no longer busy
func (cache *FileCache) MarkPathFree(path string) {
	cache.busyMutex.Lock()
	defer cache.busyMutex.Unlock()
	delete(cache.busyPaths, path)
}

// Check is a path needs a purge
func (cache *FileCache) PathNeedsPurge(path string) bool {
	cache.purgedMutex.RLock()
	defer cache.purgedMutex.RUnlock()
	return cache.purgedPaths[path]
}

// Remove the purge mark for a path. Typically called after the purge has been
// fulfilled. Returns true if the purge status was removed
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

// Mark a path that it needs a purge. A purged file will redownload and cache
// from the backend on the next request.
func (cache *FileCache) MarkPathNeedsPurge(path string) {
	cache.purgedMutex.Lock()
	defer cache.purgedMutex.Unlock()
	cache.purgedPaths[path] = true
}

// Count the entire size of tracked files in bytes from the stored
// Content-Length headers
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

// Remove a path from the cache
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

func (cache *FileCache) PathWriter(subPath string) (*os.File, error) {
	cacheTarget, err := cache.CacheFilePath(subPath)

	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(path.Dir(cacheTarget), 0755)

	if err != nil {
		return nil, err
	}

	file, err := os.Create(cacheTarget)
	return file, err
}
