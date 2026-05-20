package httpx

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func BenchmarkClientDo(b *testing.B) {
	client := NewClient(Config{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     http.Header{},
					Body:       http.NoBody,
					Request:    req,
				}, nil
			}),
		},
		UserAgent:   "seaweed-bench",
		BearerToken: "token",
	})
	query := url.Values{
		"collection": []string{"photos"},
		"limit":      []string{"100"},
	}
	header := http.Header{
		"X-Test": []string{"value"},
	}

	b.ReportAllocs()
	for b.Loop() {
		resp, err := client.Do(context.Background(), Request{
			Method:        http.MethodGet,
			URL:           "http://seaweed.test/dir/lookup",
			Query:         query,
			Header:        header,
			ContentLength: -1,
		})
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Body.Close()
	}
}

func BenchmarkClientDecodeJSON(b *testing.B) {
	client := NewClient(Config{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"count":1,"fid":"7,abc","url":"127.0.0.1:8080"}`)),
					Request:    req,
				}, nil
			}),
		},
	})
	var out struct {
		Count int    `json:"count"`
		FID   string `json:"fid"`
		URL   string `json:"url"`
	}

	b.ReportAllocs()
	for b.Loop() {
		out = struct {
			Count int    `json:"count"`
			FID   string `json:"fid"`
			URL   string `json:"url"`
		}{}
		if err := client.DecodeJSON(context.Background(), Request{
			Method:        http.MethodGet,
			URL:           "http://seaweed.test/dir/assign",
			ContentLength: -1,
		}, &out); err != nil {
			b.Fatal(err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
