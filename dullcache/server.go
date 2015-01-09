package dullcache

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
)

type errorHandler func(http.ResponseWriter, *http.Request) error

var baseUrl = "http://commondatastorage.googleapis.com"
var cacheBase = "cache"

var fileCache *FileCache

var headersToFilter = map[string]bool{"Accept-Ranges": true, "Server": true}

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
	for k, v := range headers {
		w.Header()[k] = v
	}
}

func passThrough(w http.ResponseWriter, r *http.Request) error {
	remoteRes, err := openRemote(r)

	if err != nil {
		return err
	}

	defer remoteRes.Body.Close()

	fmt.Println("Remote responded", remoteRes.Status)

	passHeaders(w, remoteRes.Header)
	copied, err := io.Copy(w, remoteRes.Body)

	if err != nil {
		fmt.Println("Copied bytes with error:", copied, err.Error())
	} else {
		fmt.Println("Copied bytes:", copied)
	}

	return nil
}

func serveAndStore(w http.ResponseWriter, r *http.Request) error {
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

	if fileCache.MarkPathBusy(r.URL.Path) {
		defer fileCache.MarkPathBusy(r.URL.Path)

		// it's now busy because of us
		cacheTarget := fileCache.CacheFilePath(r.URL.Path)
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
		log.Print("Serve and store: " + r.URL.Path)
	} else {
		log.Print("Pass through (from store): " + r.URL.Path)
	}

	passHeaders(w, remoteRes.Header)

	_, err = io.Copy(targetWriter, remoteRes.Body)

	if err != nil {
		return err
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

	_, err = io.Copy(w, file)
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
			return serveCache(w, r, headers)
		}
	}

	if fileCache.PathBusy(subPath) {
		log.Print("Pass through" + subPath)
		return passThrough(w, r)
	}

	return serveAndStore(w, r)
}

func statHandler(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprintln(w, "Cached files: ", fileCache.CountAvailablePaths(), "\n")
	return nil
}

func StartDullCache(listenTo string) error {
	fileCache = NewFileCache("cache")

	http.Handle("/stat", errorHandler(statHandler))
	http.Handle("/", errorHandler(cacheHandler))

	log.Print("Listening on: " + listenTo)
	return http.ListenAndServe(listenTo, nil)
}
