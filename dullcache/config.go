package dullcache

import (
	"encoding/json"
	"io/ioutil"
	"log"
)

const DefaultConfigFname = "dullcache.json"

type config struct {
	Address        string
	AdminAddresses []string
	CacheDir       string
}

var defaultConfig = config{
	Address:        ":9192",
	CacheDir:       "cache",
	AdminAddresses: []string{"127.0.0.1"},
}

func LoadConfig(fname string) *config {
	c := defaultConfig
	if fname == "" {
		return &c
	}

	jsonBlob, err := ioutil.ReadFile(fname)
	if err == nil {
		err = json.Unmarshal(jsonBlob, &c)

		if err != nil {
			log.Fatal("Failed parsing config: ", fname, ": ", err.Error())
		}
	} else {
		log.Print(err.Error())
	}

	return &c
}
