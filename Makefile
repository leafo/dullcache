.PHONY: install test

install:
	go install github.com/leafo/dullcache

test:
	go test -v github.com/leafo/dullcache/dullcache
