//go:build integration

package tus_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
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
	if got := strings.Join(options.ExtensionList, ","); got != "creation,creation-with-upload,termination" {
		t.Fatalf("ExtensionList = %q, want SeaweedFS TUS extensions", got)
	}
	if !options.SupportsCreation || !options.SupportsCreationWithUpload || !options.SupportsTermination {
		t.Fatalf("support flags = %+v, want SeaweedFS TUS capabilities", options)
	}
	for _, unsupported := range []string{"checksum", "creation-defer-length", "expiration", "concatenation"} {
		if slices.Contains(options.ExtensionList, unsupported) {
			t.Fatalf("ExtensionList includes unsupported extension %q: %v", unsupported, options.ExtensionList)
		}
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

func TestTUSMultiFilerUploadLocationAffinityIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeFiler(t, ctx)
	secondFilerURL := cluster.StartFiler(t, ctx)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{cluster.MasterURL},
		FilerURLs:  []string{cluster.FilerURL, secondFilerURL},
		TUSEndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	firstUpload, err := client.TUS().Create(ctx, "/tus/affinity-first.bin", tus.CreateOptions{Size: 4})
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	secondUpload, err := client.TUS().Create(ctx, "/tus/affinity-second.bin", tus.CreateOptions{Size: 4})
	if err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	assertLocationPrefix(t, firstUpload.Location, cluster.FilerURL)
	assertLocationPrefix(t, secondUpload.Location, secondFilerURL)

	firstStatus, err := client.TUS().Patch(ctx, firstUpload.Location, 0, strings.NewReader("ab"), 2, tus.PatchOptions{})
	if err != nil {
		t.Fatalf("first Patch() error = %v", err)
	}
	if firstStatus.Offset != 2 {
		t.Fatalf("first Patch offset = %d, want 2", firstStatus.Offset)
	}
	secondStatus, err := client.TUS().Patch(ctx, secondUpload.Location, 0, strings.NewReader("cd"), 2, tus.PatchOptions{})
	if err != nil {
		t.Fatalf("second Patch() error = %v", err)
	}
	if secondStatus.Offset != 2 {
		t.Fatalf("second Patch offset = %d, want 2", secondStatus.Offset)
	}

	firstStatus, err = client.TUS().Head(ctx, firstUpload.Location, tus.HeadOptions{})
	if err != nil {
		t.Fatalf("first Head() error = %v", err)
	}
	if firstStatus.Offset != 2 || firstStatus.Size != 4 {
		t.Fatalf("first Head() = %+v, want offset 2 size 4", firstStatus)
	}
	secondStatus, err = client.TUS().Head(ctx, secondUpload.Location, tus.HeadOptions{})
	if err != nil {
		t.Fatalf("second Head() error = %v", err)
	}
	if secondStatus.Offset != 2 || secondStatus.Size != 4 {
		t.Fatalf("second Head() = %+v, want offset 2 size 4", secondStatus)
	}
}

func TestTUSSeaweedFSStatusBoundariesIntegration(t *testing.T) {
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

	assertRawTusStatus(t, ctx, http.MethodPost, cluster.FilerURL+"/.tus/status/missing-version.bin", nil, map[string]string{
		"Upload-Length": "1",
	}, http.StatusPreconditionFailed)

	assertRawTusStatus(t, ctx, http.MethodPost, cluster.FilerURL+"/.tus/status/missing-length.bin", nil, map[string]string{
		"Tus-Resumable": tus.Version,
	}, http.StatusBadRequest)

	assertRawTusStatus(t, ctx, http.MethodPost, cluster.FilerURL+"/.tus/", nil, map[string]string{
		"Tus-Resumable": tus.Version,
		"Upload-Length": "1",
	}, http.StatusBadRequest)

	assertRawTusStatus(t, ctx, http.MethodPost, cluster.FilerURL+"/.tus/status/too-large.bin", nil, map[string]string{
		"Tus-Resumable": tus.Version,
		"Upload-Length": fmt.Sprintf("%d", options.MaxSize+1),
	}, http.StatusRequestEntityTooLarge)

	created, err := client.TUS().Create(ctx, "/status/wrong-content-type.bin", tus.CreateOptions{Size: 4})
	if err != nil {
		t.Fatalf("Create wrong-content-type upload error = %v", err)
	}
	assertRawTusStatus(t, ctx, http.MethodPatch, created.Location, strings.NewReader("ab"), map[string]string{
		"Tus-Resumable": tus.Version,
		"Upload-Offset": "0",
		"Content-Type":  "text/plain",
	}, http.StatusUnsupportedMediaType)

	mismatch, err := client.TUS().Create(ctx, "/status/offset-mismatch.bin", tus.CreateOptions{Size: 4})
	if err != nil {
		t.Fatalf("Create offset-mismatch upload error = %v", err)
	}
	assertRawTusStatus(t, ctx, http.MethodPatch, mismatch.Location, strings.NewReader("ab"), map[string]string{
		"Tus-Resumable": tus.Version,
		"Upload-Offset": "1",
		"Content-Type":  "application/offset+octet-stream",
	}, http.StatusConflict)

	assertRawTusStatus(t, ctx, http.MethodHead, cluster.FilerURL+"/.tus/.uploads/missing", nil, map[string]string{
		"Tus-Resumable": tus.Version,
	}, http.StatusNotFound)
	assertRawTusStatus(t, ctx, http.MethodDelete, cluster.FilerURL+"/.tus/.uploads/missing", nil, map[string]string{
		"Tus-Resumable": tus.Version,
	}, http.StatusNoContent)
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

func assertRawTusStatus(t *testing.T, ctx context.Context, method string, rawURL string, body io.Reader, header map[string]string, want int) {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		t.Fatalf("new %s %s request: %v", method, rawURL, err)
	}
	for key, value := range header {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s status = %d body %q, want %d", method, rawURL, resp.StatusCode, responseBody, want)
	}
}

func assertLocationPrefix(t *testing.T, location string, wantPrefix string) {
	t.Helper()

	if !strings.HasPrefix(location, wantPrefix+"/.tus/.uploads/") {
		t.Fatalf("location = %q, want prefix %q", location, wantPrefix+"/.tus/.uploads/")
	}
}

func isHTTPStatus(err error, status int) bool {
	var httpErr *seaweed.Error
	return errors.As(err, &httpErr) && httpErr.StatusCode == status
}
