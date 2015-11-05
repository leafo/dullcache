package dullcache

import (
	"net/http"
	"testing"
)

func getCache() *FileCache {
	return NewFileCache("test_cache")
}

func TestEmptyFileCacheCounts(t *testing.T) {
	cache := getCache()

	if 0 != cache.CountAvailablePaths() {
		t.Error("Expected available paths to be 0")
	}

	if 0 != cache.CountBusyPaths() {
		t.Error("Expected busy paths to be 0")
	}

	if 0 != cache.CountPurgedPaths() {
		t.Error("Expected purged paths to be 0")
	}
}

func TestEmptyFileCache(t *testing.T) {
	cache := getCache()
	path, _ := cache.CacheFilePath("hello/world.png")
	expected := "test_cache/3vQB7B6Nh9LkzVmtxGgw8"
	if path != expected {
		t.Error("Expected cache path to be", expected, ", got ", path)
	}
}

func TestPathAvailable(t *testing.T) {
	cache := getCache()
	available := cache.PathAvailable("hello/world.png")
	if available != nil {
		t.Fatal("didn't expect unknown path to be available")
	}

	cache.MarkPathAvailable("hello/world.png", http.Header{
		"ContentLength": []string{"1234"},
	})

	available = cache.PathAvailable("hello/world.png")

	if available == nil {
		t.Fatal("expected to get available path")
	}
}
