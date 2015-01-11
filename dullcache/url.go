package dullcache

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"strconv"
	"strings"
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
	return signer.SignURLWithExpire(method, bucket, name,
		time.Now().Add(signer.expireAfter))
}

func (signer *urlSigner) SignURLWithExpire(method, bucket, name string, expires time.Time) (string, error) {
	options := storage.SignedURLOptions{
		GoogleAccessID: signer.googleAccessID,
		PrivateKey:     signer.privateKey,
		Method:         method,
		Expires:        expires,
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

func (signer *urlSigner) VerifyURL(checkURL *url.URL) error {
	values := checkURL.Query()

	expiresStr := values.Get("Expires")
	expires, err := strconv.Atoi(expiresStr)
	if err != nil {
		return fmt.Errorf("missing expire")
	}

	// already expired, skip
	if int(time.Now().Unix()) > expires {
		return fmt.Errorf("already expired")
	}

	// need to fix the special chars issue before I can enable this
	/*
		bucket, name, err := signer.SplitBucketAndName(checkURL.Path)
		if err != nil {
			return fmt.Errorf("invalid path")
		}

		signedURLStr, err := signer.SignURLWithExpire("GET", bucket, name,
			time.Unix(int64(expires), 0))

		if err != nil {
			return fmt.Errorf("failed to calculate signature")
		}

		signedURL, err := url.Parse(signedURLStr)
		if err != nil {
			return fmt.Errorf("failed to calculate signature")
		}

		expectedSignature := signedURL.Query().Get("Signature")

		if values.Get("Signature") == expectedSignature {
			log.Print("SIGNATURES MATCH")
		} else {
			log.Print("MISMATCH SIGNATURES")
		}
	*/

	return nil
}
