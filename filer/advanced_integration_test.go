//go:build integration

package filer_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/filer"
	"github.com/lingjhf/seaweed/internal/testweed"
)

func TestFilerAdvancedIntegration(t *testing.T) {
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

	_, err = client.Filer().Put(ctx, "/advanced/source.txt", strings.NewReader("hello"), filer.WriteOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("hello")),
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	_, err = client.Filer().Append(ctx, "/advanced/source.txt", strings.NewReader("-world"), filer.AppendOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("-world")),
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	assertAdvancedContent(t, ctx, client, "/advanced/source.txt", "hello-world")

	if err := client.Filer().Mkdir(ctx, "/advanced/uploads"); err != nil {
		t.Fatalf("Mkdir(upload dir) error = %v", err)
	}
	_, err = client.Filer().UploadMultipart(ctx, "/advanced/uploads/multipart.txt", strings.NewReader("multipart-data"), filer.MultipartUploadOptions{
		FileContentType: "text/plain",
		SeaweedHeaders: map[string]string{
			"Origin": "multipart",
		},
	})
	if err != nil {
		t.Fatalf("UploadMultipart() error = %v", err)
	}
	assertAdvancedContent(t, ctx, client, "/advanced/uploads/multipart.txt", "multipart-data")
	multipartHead, err := client.Filer().Head(ctx, "/advanced/uploads/multipart.txt")
	if err != nil {
		t.Fatalf("Head(multipart) error = %v", err)
	}
	if multipartHead.Header.Get("Seaweed-Origin") != "multipart" {
		t.Fatalf("Seaweed-Origin = %q, want multipart", multipartHead.Header.Get("Seaweed-Origin"))
	}
	multipartEntry, err := client.Filer().Stat(ctx, "/advanced/uploads/multipart.txt", filer.StatOptions{})
	if err != nil {
		t.Fatalf("Stat(multipart) error = %v", err)
	}
	if multipartEntry.FullPath != "/advanced/uploads/multipart.txt" || multipartEntry.FileSize != int64(len("multipart-data")) {
		t.Fatalf("multipart stat = %+v, want uploaded path and size", multipartEntry)
	}

	if err := client.Filer().Copy(ctx, "/advanced/source.txt", "/advanced/copy.txt"); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	assertAdvancedContent(t, ctx, client, "/advanced/copy.txt", "hello-world")

	if err := client.Filer().Move(ctx, "/advanced/copy.txt", "/advanced/moved.txt"); err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	assertAdvancedContent(t, ctx, client, "/advanced/moved.txt", "hello-world")

	if err := client.Filer().SetTags(ctx, "/advanced/moved.txt", map[string]string{"Project": "sdk"}); err != nil {
		t.Fatalf("SetTags() error = %v", err)
	}
	head, err := client.Filer().Head(ctx, "/advanced/moved.txt")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if head.Header.Get("Seaweed-Project") != "sdk" {
		t.Fatalf("Seaweed-Project = %q, want sdk", head.Header.Get("Seaweed-Project"))
	}
	tags, err := client.Filer().Tags(ctx, "/advanced/moved.txt")
	if err != nil {
		t.Fatalf("Tags() error = %v", err)
	}
	if tags["Project"] != "sdk" {
		t.Fatalf("Tags()[Project] = %q, want sdk", tags["Project"])
	}
	walked := []string{}
	err = client.Filer().Walk(ctx, "/advanced", filer.WalkOptions{Limit: 1}, func(entry filer.Entry) error {
		walked = append(walked, entry.FullPath)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if !slices.Contains(walked, "/advanced/source.txt") || !slices.Contains(walked, "/advanced/moved.txt") {
		t.Fatalf("Walk() paths = %#v", walked)
	}
	if err := client.Filer().DeleteTags(ctx, "/advanced/moved.txt", "Project"); err != nil {
		t.Fatalf("DeleteTags() error = %v", err)
	}
	head, err = client.Filer().Head(ctx, "/advanced/moved.txt")
	if err != nil {
		t.Fatalf("Head after delete tags error = %v", err)
	}
	if head.Header.Get("Seaweed-Project") != "" {
		t.Fatalf("Seaweed-Project after delete = %q, want empty", head.Header.Get("Seaweed-Project"))
	}
}

func assertAdvancedContent(t *testing.T, ctx context.Context, client *seaweed.Client, path string, want string) {
	t.Helper()

	resp, err := client.Filer().Get(ctx, path, filer.GetOptions{})
	if err != nil {
		t.Fatalf("Get(%s) error = %v", path, err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(body) != want {
		t.Fatalf("%s content = %q, want %q", path, body, want)
	}
}
