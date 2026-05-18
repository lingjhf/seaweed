package blob

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lingjhf/seaweed/master"
)

func BenchmarkCachedVolumeClientFor(b *testing.B) {
	volumeServer := httptest.NewServer(http.NotFoundHandler())
	defer volumeServer.Close()

	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]string{
				{
					"url": strings.TrimPrefix(volumeServer.URL, "http://"),
				},
			},
		})
	}))
	defer masterServer.Close()

	masterClient, err := master.New(master.Config{
		BaseURLs:   []string{masterServer.URL},
		HTTPClient: masterServer.Client(),
	})
	if err != nil {
		b.Fatal(err)
	}
	client, err := New(Config{
		Master:     masterClient,
		HTTPClient: masterServer.Client(),
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	if _, err := client.volumeClientFor(ctx, "9,abc"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		volumeClient, err := client.volumeClientFor(ctx, "9,abc")
		if err != nil {
			b.Fatal(err)
		}
		if volumeClient == nil {
			b.Fatal("volume client is nil")
		}
	}
}
