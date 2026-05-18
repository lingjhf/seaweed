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

func TestFilerAdvancedIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeFiler(t, ctx)
	client, err := seaweed.New(seaweed.Config{
		MasterURL: cluster.MasterURL,
		FilerURL:  cluster.FilerURL,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Filer().Put(ctx, "/advanced/source.txt", strings.NewReader("hello"), filer.PutOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("hello")),
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	_, err = client.Filer().Append(ctx, "/advanced/source.txt", strings.NewReader("-world"), filer.PutOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("-world")),
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	assertAdvancedContent(t, ctx, client, "/advanced/source.txt", "hello-world")

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
	header, err := client.Filer().Head(ctx, "/advanced/moved.txt")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if header.Get("Seaweed-Project") != "sdk" {
		t.Fatalf("Seaweed-Project = %q, want sdk", header.Get("Seaweed-Project"))
	}
	if err := client.Filer().DeleteTags(ctx, "/advanced/moved.txt", "Project"); err != nil {
		t.Fatalf("DeleteTags() error = %v", err)
	}
	header, err = client.Filer().Head(ctx, "/advanced/moved.txt")
	if err != nil {
		t.Fatalf("Head after delete tags error = %v", err)
	}
	if header.Get("Seaweed-Project") != "" {
		t.Fatalf("Seaweed-Project after delete = %q, want empty", header.Get("Seaweed-Project"))
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
