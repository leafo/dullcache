package dullcache

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
)

type errorHandler func(http.ResponseWriter, *http.Request) error

var baseUrl = "http://commondatastorage.googleapis.com"
var cacheBase = "cache"

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

func cacheHandler(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" {
		return fmt.Errorf("only GET allowed")
	}

	return serveAndStore(w, r)
	// return passThrough(w, r)
}

func StartDullCache(listenTo string) error {
	http.Handle("/", errorHandler(cacheHandler))

	log.Print("Listening on: " + listenTo)
	return http.ListenAndServe(listenTo, nil)
}
