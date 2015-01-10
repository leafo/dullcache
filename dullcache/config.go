package dullcache

import (
	"encoding/json"
	"io/ioutil"
	"log"
)

const DefaultConfigFname = "dullcache.json"

type Config struct {
	Address                     string
	AdminAddresses              []string
	CacheDir                    string
	GoogleAccessID              string
	GoogleStoragePrivateKeyPath string
}

var defaultConfig = Config{
	Address:                     ":9192",
	CacheDir:                    "cache",
	AdminAddresses:              []string{"127.0.0.1", "[::1]"},
	GoogleAccessID:              "",
	GoogleStoragePrivateKeyPath: "",
}

func LoadConfig(fname string) *Config {
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
