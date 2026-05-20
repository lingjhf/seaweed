//go:build integration

package tus_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/filer"
	"github.com/lingjhf/seaweed/internal/testweed"
	"github.com/lingjhf/seaweed/tus"
)

func TestTUSUploadResumeTerminateIntegration(t *testing.T) {
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

	options, err := client.TUS().Options(ctx, tus.OptionsOptions{})
	if err != nil {
		t.Fatalf("Options() error = %v", err)
	}
	if options.Version != tus.Version {
		t.Fatalf("Version = %q, want %s", options.Version, tus.Version)
	}

	uploaded, err := client.TUS().Upload(ctx, "/tus/basic.txt", strings.NewReader("basic-data"), tus.UploadOptions{
		Size:      int64(len("basic-data")),
		ChunkSize: 4,
		Metadata: map[string]string{
			"filename": "basic.txt",
		},
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if uploaded.Offset != int64(len("basic-data")) {
		t.Fatalf("Upload offset = %d", uploaded.Offset)
	}
	assertFilerContent(t, ctx, client, "/tus/basic.txt", "basic-data")

	resumeData := []byte("resume-data")
	created, err := client.TUS().Create(ctx, "/tus/resume.txt", tus.CreateOptions{
		Size: int64(len(resumeData)),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	first := int64(6)
	status, err := client.TUS().Patch(ctx, created.Location, 0, bytes.NewReader(resumeData[:first]), first, tus.PatchOptions{})
	if err != nil {
		t.Fatalf("Patch() error = %v", err)
	}
	if status.Offset != first {
		t.Fatalf("Patch offset = %d, want %d", status.Offset, first)
	}
	status, err = client.TUS().Resume(ctx, created.Location, bytes.NewReader(resumeData), tus.ResumeOptions{ChunkSize: 3})
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if status.Offset != int64(len(resumeData)) {
		t.Fatalf("Resume offset = %d", status.Offset)
	}
	assertFilerContent(t, ctx, client, "/tus/resume.txt", string(resumeData))

	cancelled, err := client.TUS().Create(ctx, "/tus/cancel.txt", tus.CreateOptions{Size: 5})
	if err != nil {
		t.Fatalf("Create cancel upload error = %v", err)
	}
	if err := client.TUS().Terminate(ctx, cancelled.Location, tus.TerminateOptions{}); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	_, err = client.TUS().Head(ctx, cancelled.Location, tus.HeadOptions{})
	if !isHTTPStatus(err, http.StatusNotFound) {
		t.Fatalf("Head after terminate error = %v, want 404", err)
	}
}

func assertFilerContent(t *testing.T, ctx context.Context, client *seaweed.Client, path string, want string) {
	t.Helper()

	resp, err := client.Filer().Get(ctx, path, filer.GetOptions{})
	if err != nil {
		t.Fatalf("Filer.Get(%s) error = %v", path, err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read filer content: %v", err)
	}
	if string(body) != want {
		t.Fatalf("content = %q, want %q", body, want)
	}
}

func isHTTPStatus(err error, status int) bool {
	var httpErr *seaweed.Error
	return errors.As(err, &httpErr) && httpErr.StatusCode == status
}
