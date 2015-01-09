package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/leafo/dullcache/dullcache"
)

var (
	configFname string
)

func init() {
	flag.StringVar(&configFname, "config",
		dullcache.DefaultConfigFname, "Path to json config")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dullcache [OPTIONS]\n\nOptions:\n")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()
	config := dullcache.LoadConfig(configFname)
	dullcache.StartDullCache(config.Address)
}
