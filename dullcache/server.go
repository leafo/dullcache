package dullcache

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync/atomic"
)

type errorHandler func(http.ResponseWriter, *http.Request) error

var baseUrl = "http://commondatastorage.googleapis.com"
var cacheBase = "cache"

var fileCache *FileCache

var headersToFilter = map[string]bool{"Accept-Ranges": true, "Server": true}

var serverStats struct {
	bytesFetched int64
	bytesSent    int64
	fastHits     int64
	checkedHits  int64
	passes       int64
	stores       int64
}

func (fn errorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func openRemote(r *http.Request) (*http.Response, error) {
	reqUrl := *r.URL
	fetchUrl := baseUrl + reqUrl.Path

	if reqUrl.RawQuery != "" {
		fetchUrl += "?" + reqUrl.RawQuery
	}

	log.Print("Remote GET: " + fetchUrl)
	return http.Get(fetchUrl)
}

func filterHeaders(headers http.Header) http.Header {
	filtered := http.Header{}

	for k, v := range headers {
		if !headersToFilter[k] {
			filtered[k] = v
		}
	}

	return filtered
}

func headPath(subPath string) (http.Header, error) {
	log.Print("Remote HEAD: " + subPath)

	res, err := http.Head(baseUrl + subPath)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("failed to head file")
	}

	return filterHeaders(res.Header), err
}

func passHeaders(w http.ResponseWriter, headers http.Header) {
	for k, v := range filterHeaders(headers) {
		w.Header()[k] = v
	}
}

func passThrough(w http.ResponseWriter, r *http.Request) error {
	remoteRes, err := openRemote(r)

	if err != nil {
		return err
	}

	defer remoteRes.Body.Close()

	passHeaders(w, remoteRes.Header)
	copied, err := io.Copy(w, remoteRes.Body)

	atomic.AddInt64(&serverStats.bytesFetched, copied)
	atomic.AddInt64(&serverStats.bytesSent, copied)

	return nil
}

func serveAndStore(w http.ResponseWriter, r *http.Request) error {
	subPath := r.URL.Path
	remoteRes, err := openRemote(r)

	if err != nil {
		return err
	}

	defer remoteRes.Body.Close()

	if remoteRes.StatusCode != 200 {
		passHeaders(w, remoteRes.Header)
		_, err = io.Copy(w, remoteRes.Body)
		return err
	}

	if err != nil {
		return err
	}

	var targetWriter io.Writer = w

	writingCache := fileCache.MarkPathBusy(subPath)

	if writingCache {
		defer fileCache.MarkPathFree(subPath)

		// it's now busy because of us
		cacheTarget := fileCache.CacheFilePath(subPath)
		err = os.MkdirAll(path.Dir(cacheTarget), 0755)

		if err != nil {
			return err
		}

		file, err := os.Create(cacheTarget)

		if err != nil {
			return err
		}

		defer file.Close()

		targetWriter = io.MultiWriter(file, targetWriter)
		log.Print("Serve and store: " + subPath)
		atomic.AddInt64(&serverStats.stores, 1)
	} else {
		log.Print("Pass through (from store): " + subPath)
		atomic.AddInt64(&serverStats.passes, 1)
	}

	passHeaders(w, remoteRes.Header)

	copied, err := io.Copy(targetWriter, remoteRes.Body)

	atomic.AddInt64(&serverStats.bytesFetched, copied)
	atomic.AddInt64(&serverStats.bytesSent, copied)

	if err != nil {
		log.Print("Aborted writing cache: " + subPath)
		// can't render normal error handler because we already set headers, so do
		// nothing
		return nil
	}

	if writingCache {
		fileCache.MarkPathAvailable(subPath, filterHeaders(remoteRes.Header))
		log.Print("Cache stored" + subPath)
	}

	return nil
}

func serveCache(w http.ResponseWriter, r *http.Request, fileHeaders http.Header) error {
	file, err := os.Open(fileCache.CacheFilePath(r.URL.Path))

	if err != nil {
		return err
	}

	defer file.Close()

	passHeaders(w, fileHeaders)

	copied, err := io.Copy(w, file)

	atomic.AddInt64(&serverStats.bytesSent, copied)

	return err
}

func cacheHandler(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" {
		return fmt.Errorf("only GET allowed")
	}

	subPath := r.URL.Path

	availableHeaders := fileCache.PathAvailable(subPath)
	if availableHeaders != nil {
		log.Print("From cache quick: " + subPath)
		atomic.AddInt64(&serverStats.fastHits, 1)
		return serveCache(w, r, availableHeaders)
	}

	size, err := fileCache.PathMaybeAvailable(subPath)

	if err != nil {
		return err
	}

	if size > 0 {
		headers, err := headPath(subPath)
		contentLenStr := headers.Get("Content-Length")
		contentLen, err := strconv.Atoi(contentLenStr)

		if err != nil {
			return err
		}

		if int64(contentLen) == size {
			fileCache.MarkPathAvailable(subPath, headers)
			log.Print("From cache checked: " + subPath)
			atomic.AddInt64(&serverStats.checkedHits, 1)
			return serveCache(w, r, headers)
		}
	}

	if fileCache.PathBusy(subPath) {
		log.Print("Pass through" + subPath)
		atomic.AddInt64(&serverStats.passes, 1)
		return passThrough(w, r)
	}

	return serveAndStore(w, r)
}

func statHandler(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprintln(w, "Available paths: ", fileCache.CountAvailablePaths())
	fmt.Fprintln(w, "Busy paths: ", fileCache.CountBusyPaths())
	fmt.Fprintln(w, "Fast hits: ", serverStats.fastHits)
	fmt.Fprintln(w, "Checked hits: ", serverStats.checkedHits)
	fmt.Fprintln(w, "Passes: ", serverStats.passes)
	fmt.Fprintln(w, "Stores: ", serverStats.stores)
	return nil
}

func StartDullCache(listenTo string) error {
	fileCache = NewFileCache("cache")

	http.Handle("/stat", errorHandler(statHandler))
	http.Handle("/", errorHandler(cacheHandler))

	log.Print("Listening on: " + listenTo)
	return http.ListenAndServe(listenTo, nil)
}
