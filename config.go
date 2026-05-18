package seaweed

import (
	"net/http"
	"time"

	"github.com/lingjhf/seaweed/internal/httpx"
)

type RetryPolicy = httpx.RetryPolicy

type Config struct {
	MasterURL       string
	VolumeURL       string
	FilerURL        string
	TusBasePath     string
	S3URL           string
	IAMURL          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	BearerToken     string
	UserAgent       string
	UsePublicURLs   bool
	Retry           RetryPolicy
}

type Option func(*options)

type options struct {
	httpClient *http.Client
}

func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		o.httpClient = client
	}
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		Wait:        100 * time.Millisecond,
	}
}
