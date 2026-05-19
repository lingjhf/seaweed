//go:build integration

package filer_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/filer"
	"github.com/lingjhf/seaweed/internal/testweed"
)

func TestFilerCoreIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeFiler(t, ctx)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{cluster.MasterURL},
		FilerURLs:  []string{cluster.FilerURL},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := client.Filer().Mkdir(ctx, "/empty", filer.MkdirOptions{}); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	_, err = client.Filer().Put(ctx, "/docs/report.txt", strings.NewReader("filer-data"), filer.WriteOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("filer-data")),
		SeaweedHeaders: map[string]string{
			"Owner": "sdk",
		},
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	resp, err := client.Filer().Get(ctx, "/docs/report.txt", filer.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "filer-data" {
		t.Fatalf("body = %q, want filer-data", body)
	}

	head, err := client.Filer().Head(ctx, "/docs/report.txt", filer.HeadOptions{})
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if head.Header.Get("Seaweed-Owner") != "sdk" {
		t.Fatalf("Seaweed-Owner = %q, want sdk", head.Header.Get("Seaweed-Owner"))
	}

	entry, err := client.Filer().Stat(ctx, "/docs/report.txt", filer.StatOptions{})
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if entry.FullPath != "/docs/report.txt" {
		t.Fatalf("FullPath = %q, want /docs/report.txt", entry.FullPath)
	}

	list, err := client.Filer().ListPage(ctx, "/docs", filer.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list.Entries) != 1 || list.Entries[0].FullPath != "/docs/report.txt" {
		t.Fatalf("Entries = %#v", list.Entries)
	}

	if err := client.Filer().Delete(ctx, "/docs/report.txt", filer.DeleteOptions{}); err != nil {
		t.Fatalf("Delete(file) error = %v", err)
	}
	if err := client.Filer().Delete(ctx, "/empty", filer.DeleteOptions{Recursive: true}); err != nil {
		t.Fatalf("Delete(dir) error = %v", err)
	}
}
