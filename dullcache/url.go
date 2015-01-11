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
		expireAfter:    time.Duration(5) * time.Minute,
	}, nil
}

func (signer *urlSigner) SignURL(method, bucket, name string) (string, error) {
	options := storage.SignedURLOptions{
		GoogleAccessID: signer.googleAccessID,
		PrivateKey:     signer.privateKey,
		Method:         method,
		Expires:        time.Now().Add(signer.expireAfter),
	}

	return storage.SignedURL(bucket, name, &options)
}

func (signer *urlSigner) SplitBucketAndName(path string) (string, string, error) {
	splits := strings.SplitN(path, "/", 3)

	if len(splits) == 3 {
		return splits[1], splits[2], nil
	}

	return "", "", fmt.Errorf("failed to split bucket and name")
}

