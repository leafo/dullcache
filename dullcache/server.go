package dullcache

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

type errorHandler func(http.ResponseWriter, *http.Request) error

var baseUrl = "http://commondatastorage.googleapis.com"

func (fn errorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func cacheHandler(w http.ResponseWriter, r *http.Request) error {
	reqUrl := *r.URL

	fetchUrl := baseUrl + reqUrl.Path + "?" + reqUrl.RawQuery

	fmt.Println("Fetching", fetchUrl)
	res, err := http.Get(fetchUrl)

	if err != nil {
		return err
	}

	fmt.Println("Server responded", res.Status)

	for k, v := range res.Header {
		w.Header()[k] = v
	}

	copied, err := io.Copy(w, res.Body)
	if err != nil {
		fmt.Println("Copied bytes with error:", copied, err.Error())
	} else {
		fmt.Println("Copied bytes:", copied)
	}

	defer res.Body.Close()

	return nil
}

func StartDullCache(listenTo string) error {
	http.Handle("/", errorHandler(cacheHandler))

	log.Print("Listening on: " + listenTo)
	return http.ListenAndServe(listenTo, nil)
}
