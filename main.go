package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/leafo/dullcache/dullcache"
)

var (
	configFname   string
	logTimestamps bool
)

const version = "1.0"

func init() {
	flag.StringVar(&configFname, "config",
		dullcache.DefaultConfigFname, "Path to json config")

	flag.BoolVar(&logTimestamps, "timestamps",
		true, "Include timestamps in log messages")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "dullcache version %v\n", version)
		fmt.Fprintf(os.Stderr, "Usage: dullcache [OPTIONS]\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	if !logTimestamps {
		log.SetFlags(0)
	}

	config := dullcache.LoadConfig(configFname)
	err := dullcache.StartDullCache(config)

	if err != nil {
		log.Fatal(err.Error())
	}
}
