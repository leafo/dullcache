package dullcache

import (
	"io/ioutil"
	"time"

	"google.golang.org/cloud/storage"
)

type urlSigner struct {
	privateKey     []byte
	googleAccessID string
	expireAfter    time.Duration
}

func NewURLSigner(googleAccessID, privateKeyPath string) (*urlSigner, error) {
	pemBytes, err := ioutil.ReadFile(privateKeyPath)

	if err != nil {
		return nil, err
	}

	return &urlSigner{
		privateKey:     pemBytes,
		googleAccessID: googleAccessID,
		expireAfter:    time.Duration(20) * time.Second,
	}, nil
}

func (signer *urlSigner) SignUrl(method, bucket, name string) (string, error) {
	options := storage.SignedURLOptions{
		GoogleAccessID: signer.googleAccessID,
		PrivateKey:     signer.privateKey,
		Method:         method,
		Expires:        time.Now().Add(signer.expireAfter),
	}

	return storage.SignedURL(bucket, name, &options)
}
