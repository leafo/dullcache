package dullcache

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/cupcake/mannersagain"
	"github.com/dustin/go-humanize"
)

type errorHandler func(http.ResponseWriter, *http.Request) error
type adminHandler func(http.ResponseWriter, *http.Request) error

var fileCache *FileCache
var config *Config
var headURLSigner *urlSigner

var headersToFilter = map[string]bool{"Accept-Ranges": true, "Server": true}

var stats *serverStats

func calculateSpeedKbs(copied int64, elapsed time.Duration) int64 {
	return int64(float64(copied) / float64(elapsed) * float64(time.Second) / 1024)
}

func (fn errorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (fn adminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !authAdminRequest(r) {
		http.Error(w, "Invalid request", 500)
	}

	if err := fn(w, r); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func authAdminRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)

	if err != nil {
		return false
	}

	for _, allowed := range config.AdminAddresses {
		if allowed == host {
			return true
		}
	}

	return false
}

func openRemote(r *http.Request) (*http.Response, error) {
	fetchUrl := config.BaseURL + r.RequestURI
	log.Print("Remote GET: ", r.URL.Path)
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
	headURL := config.BaseURL + subPath
	if headURLSigner != nil {
		bucket, name, err := headURLSigner.SplitBucketAndName(subPath)
		if err == nil {
			// TODO: this generates bad urls for names with symbols in them
			headURL, err = headURLSigner.SignURL("HEAD", bucket, name)
			if err != nil {
				return nil, err
			}
		}
	}

	log.Print("Remote HEAD: ", subPath)
	res, err := http.Head(headURL)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("failed to head file: %v", res.StatusCode)
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

	stats.incrActivePath(r.URL.Path, 1)
	copied, err := io.Copy(w, remoteRes.Body)
	stats.incrActivePath(r.URL.Path, -1)

	stats.incrBytesFetched(uint64(copied))
	stats.incrBytesSent(uint64(copied))

	if err == nil {
		stats.incrSizeDist(uint64(copied))
	}

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
		stats.incrActivePath(subPath, 1)
		_, err = io.Copy(w, remoteRes.Body)
		stats.incrActivePath(subPath, -1)
		return err
	}

	if err != nil {
		return err
	}

	var targetWriter io.Writer = w

	writingCache := fileCache.MarkPathBusy(subPath)
	needsPurge := false

	if writingCache {
		defer fileCache.MarkPathFree(subPath)
		needsPurge = fileCache.PathNeedsPurge(subPath)

		file, err := fileCache.PathWriter(subPath)

		if err != nil {
			return err
		}

		defer file.Close()

		targetWriter = io.MultiWriter(file, targetWriter)
		log.Print("Serve and store: ", subPath)
		stats.incrStores(1)
	} else {
		log.Print("Pass through (from store): ", subPath)
		stats.incrPasses(1)
	}

	passHeaders(w, remoteRes.Header)

	stats.incrActivePath(subPath, 1)
	start := time.Now()
	copied, err := io.Copy(targetWriter, remoteRes.Body)
	elapsed := time.Since(start)
	stats.incrActivePath(subPath, -1)

	log.Print("Transfered ", subPath, " ",
		calculateSpeedKbs(copied, elapsed), " KB/s ", r.RemoteAddr)

	stats.incrBytesFetched(uint64(copied))
	stats.incrBytesSent(uint64(copied))

	if err == nil {
		stats.incrSizeDist(uint64(copied))
	}

	if err != nil {
		log.Print("Aborted writing cache: ", subPath)
		// can't render normal error handler because we already set headers, so do
		// nothing
		return nil
	}

	if writingCache {
		fileCache.MarkPathAvailable(subPath, filterHeaders(remoteRes.Header))
		fileCache.accessList.AccessPath(subPath)
		log.Print("Cache stored: ", subPath)
		if needsPurge {
			fileCache.ReleasePathPurge(subPath)
		}
	}

	return nil
}

func serveCache(w http.ResponseWriter, r *http.Request, fileHeaders http.Header) error {
	if headURLSigner != nil {
		err := headURLSigner.VerifyURL(r.URL)

		if err != nil {
			stats.incrPasses(1)
			log.Print("Passing unverifiable URL: ", r.URL.Path)
			return passThrough(w, r)
		}
	}

	filePath, err := fileCache.CacheFilePath(r.URL.Path)

	if err != nil {
		return err
	}

	file, err := os.Open(filePath)

	if err != nil {
		return err
	}

	defer file.Close()

	passHeaders(w, fileHeaders)

	stats.incrActivePath(r.URL.Path, 1)
	start := time.Now()
	copied, err := io.Copy(w, file)
	elapsed := time.Since(start)
	stats.incrActivePath(r.URL.Path, -1)

	log.Print("Transfered ", r.URL.Path, " ",
		calculateSpeedKbs(copied, elapsed), " KB/s ", r.RemoteAddr)

	stats.incrBytesSent(uint64(copied))

	if err == nil {
		stats.incrSizeDist(uint64(copied))
		fileCache.accessList.AccessPath(r.URL.Path)
	}

	return nil
}

func purgeHandler(w http.ResponseWriter, r *http.Request) error {
	if !authAdminRequest(r) {
		log.Print("Unauthorized purge attempt: ", r.URL.Path)
		return fmt.Errorf("unauthorized")
	}

	log.Print("Purging: ", r.URL.Path)
	fileCache.MarkPathNeedsPurge(r.URL.Path)
	return nil
}

func cacheHandler(w http.ResponseWriter, r *http.Request) error {
	if r.Method == "DELETE" {
		return purgeHandler(w, r)
	}

	if r.Method != "GET" {
		return fmt.Errorf("only GET allowed")
	}

	subPath := r.URL.Path

	if subPath == "/" {
		return nil
	}

	if !fileCache.PathNeedsPurge(subPath) {
		availableHeaders := fileCache.PathAvailable(subPath)
		if availableHeaders != nil {
			log.Print("From cache quick: " + subPath)
			stats.incrFastHits(1)
			return serveCache(w, r, availableHeaders)
		}

		size, err := fileCache.PathMaybeAvailable(subPath)

		if err != nil {
			return err
		}

		if size > 0 {
			headers, err := headPath(subPath)

			if err == nil {
				contentLenStr := headers.Get("Content-Length")
				contentLen, err := strconv.Atoi(contentLenStr)

				if err == nil {
					if int64(contentLen) == size {
						fileCache.MarkPathAvailable(subPath, headers)
						log.Print("From cache checked: ", subPath)
						stats.incrCheckedHits(1)
						return serveCache(w, r, headers)
					}
				}
			} else {
				log.Print("Warning, failed to HEAD path: ", subPath)
			}
		}
	}

	if fileCache.PathBusy(subPath) {
		log.Print("Pass through: ", subPath)
		stats.incrPasses(1)
		return passThrough(w, r)
	}

	return serveAndStore(w, r)
}

func statHandler(w http.ResponseWriter, r *http.Request) error {
	if r.Method == "HEAD" {
		return nil
	}

	stats.RLock()
	defer stats.RUnlock()

	fmt.Fprintln(w, "Available paths: ", fileCache.CountAvailablePaths())
	fmt.Fprintln(w, "Busy paths: ", fileCache.CountBusyPaths())
	fmt.Fprintln(w, "Purged paths: ", fileCache.CountPurgedPaths())
	fmt.Fprintln(w, "Fast hits: ", stats.fastHits)
	fmt.Fprintln(w, "Checked hits: ", stats.checkedHits)
	fmt.Fprintln(w, "Passes: ", stats.passes)
	fmt.Fprintln(w, "Stores: ", stats.stores)
	fmt.Fprintln(w, "Active transfers: ", stats.countActivePaths())
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Bytes fetched: ", humanize.Bytes(stats.bytesFetched))
	fmt.Fprintln(w, "Bytes sent: ", humanize.Bytes(stats.bytesSent))

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Size dist")
	fmt.Fprintln(w, "=========")
	for _, size := range sizeDistsMB {
		fmt.Fprintln(w, size, "MB", stats.sizeDist[size])
	}

	return nil
}

func statActiveHandler(w http.ResponseWriter, r *http.Request) error {
	stats.RLock()
	defer stats.RUnlock()
	for path, count := range stats.activePaths {
		fmt.Fprintln(w, humanize.Comma(count), path)
	}
	return nil
}

func adminListHandler(w http.ResponseWriter, r *http.Request) error {
	fileCache.availableMutex.RLock()
	defer fileCache.availableMutex.RUnlock()

	for path := range fileCache.availablePaths {
		fmt.Fprintln(w, path)
	}

	return nil
}

func adminAccessListHandler(w http.ResponseWriter, r *http.Request) error {
	fileCache.accessList.mutex.RLock()
	defer fileCache.accessList.mutex.RUnlock()

	iter := fileCache.accessList.ordered.Iterator()

	for iter.Next() {
		path := iter.Key().(string)
		time := fileCache.accessList.pathTimes[path]

		fmt.Fprintf(w, "%v %v\n", time, path)
	}

	return nil
}

func adminFileListHandler(w http.ResponseWriter, r *http.Request) error {
	fileCache.availableMutex.RLock()
	defer fileCache.availableMutex.RUnlock()

	for path := range fileCache.availablePaths {
		fname, err := fileCache.CacheFilePath(path)
		if err != nil {
			return err
		}

		fmt.Fprintf(w, "%v %v\n", fname, path)
	}

	return nil
}

func adminStatPath(w http.ResponseWriter, r *http.Request) error {
	values := r.URL.Query()
	path := values.Get("path")
	if path == "" {
		return fmt.Errorf("missing path to stat")
	}

	fileCache.availableMutex.RLock()
	defer fileCache.availableMutex.RUnlock()

	headers, found := fileCache.availablePaths[path]

	if !found {
		return fmt.Errorf("path is not available")
	}

	out, err := json.MarshalIndent(headers, "", "  ")

	if err != nil {
		return err
	}

	fmt.Fprintln(w, string(out))

	return nil
}

func adminDeletePath(w http.ResponseWriter, r *http.Request) error {
	values := r.URL.Query()
	path := values.Get("path")
	if path == "" {
		return fmt.Errorf("missing path to delete")
	}

	log.Print("Delete", path)

	return fileCache.DeletePath(path)
}

func adminAvailableSize(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprintln(w, fileCache.TrackedSize())
	return nil
}

func StartDullCache(_config *Config) error {
	fileCache = NewFileCache("cache")
	config = _config
	if config.GoogleAccessID != "" && config.GoogleStoragePrivateKeyPath != "" {
		signer, err := NewURLSigner(config.GoogleAccessID, config.GoogleStoragePrivateKeyPath)
		if err != nil {
			log.Print("Warning: failed to create URL signer: ", err)
		}
		headURLSigner = signer
	}

	stats = newServerStats()

	http.DefaultClient.Timeout = time.Duration(4) * time.Hour

	http.Handle("/stat/active", errorHandler(statActiveHandler))
	http.Handle("/stat", errorHandler(statHandler))
	http.Handle("/", errorHandler(cacheHandler))

	http.Handle("/admin/list/paths", adminHandler(adminListHandler))
	http.Handle("/admin/list/access-times", adminHandler(adminAccessListHandler))
	http.Handle("/admin/list/fnames", adminHandler(adminFileListHandler))

	http.Handle("/admin/path-headers", adminHandler(adminStatPath))
	http.Handle("/admin/delete-path", adminHandler(adminDeletePath))
	http.Handle("/admin/available-size", adminHandler(adminAvailableSize))

	return mannersagain.ListenAndServe(config.Address, nil)
}
