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

	fmt.Println("Fetching", fetchUrl)
	return http.Get(fetchUrl)
}

func filterHeaders(headers *http.Header) {
	for k, _ := range *headers {
		if headersToFilter[k] {
			headers.Del(k)
		}
	}
}

func headPath(subPath string) (*http.Header, error) {
	log.Print("HEAD " + subPath)

	res, err := http.Head(baseUrl + subPath)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("failed to head file")
	}

	headerCopy := http.Header{}
	for k, v := range res.Header {
		headerCopy[k] = v
	}

	filterHeaders(&headerCopy)

	return &headerCopy, err
}

func passHeaders(w http.ResponseWriter, remoteRes *http.Response) {
	for k, v := range remoteRes.Header {
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

	passHeaders(w, remoteRes)
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
		passHeaders(w, remoteRes)
		_, err = io.Copy(w, remoteRes.Body)
		return err
	}

	if err != nil {
		return err
	}

	cacheTarget := cacheBase + r.URL.Path
	err = os.MkdirAll(path.Dir(cacheTarget), 0755)

	if err != nil {
		return err
	}

	file, err := os.Create(cacheTarget)

	if err != nil {
		return err
	}

	defer file.Close()
	fmt.Println("Writing cache", cacheTarget)

	multi := io.MultiWriter(file, w)

	passHeaders(w, remoteRes)

	_, err = io.Copy(multi, remoteRes.Body)

	if err != nil {
		return err
	}

	return nil
}

func serveCache(w http.ResponseWriter, r *http.Request, fileHeaders *http.Header) error {
	w.Write([]byte("Serve from cache...\n"))
	return nil
}

func cacheHandler(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" {
		return fmt.Errorf("only GET allowed")
	}

	subPath := r.URL.Path

	availableHeaders := fileCache.PathAvailable(subPath)
	if availableHeaders != nil {
		log.Print("Path is available from memory: " + subPath)
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
			log.Print("Path is available from disk: " + subPath)
			fileCache.MarkPathAvailable(subPath, headers)
			return serveCache(w, r, headers)
		}
	}

	return serveAndStore(w, r)

	// return passThrough(w, r)
}

func statHandler(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprintln(w, "Cached files: ", fileCache.CountPathsCached(), "\n")
	return nil
}

func StartDullCache(listenTo string) error {
	fileCache = NewFileCache("cache")

	http.Handle("/stat", errorHandler(statHandler))
	http.Handle("/", errorHandler(cacheHandler))

	log.Print("Listening on: " + listenTo)
	return http.ListenAndServe(listenTo, nil)
}
