package testweed

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type Cluster struct {
	MasterURL string
	VolumeURL string

	dataDir string
	cmds    []*exec.Cmd
	logs    []string
}

func StartMasterVolume(t *testing.T, ctx context.Context) *Cluster {
	t.Helper()

	weed := findWeedBinary(t)
	dataDir := t.TempDir()
	masterPort, masterGRPCPort := freePortPair(t)
	volumePort, volumeGRPCPort := freePortPair(t)

	cluster := &Cluster{
		MasterURL: fmt.Sprintf("http://127.0.0.1:%d", masterPort),
		VolumeURL: fmt.Sprintf("http://127.0.0.1:%d", volumePort),
		dataDir:   dataDir,
	}

	masterDir := filepath.Join(dataDir, "master")
	volumeDir := filepath.Join(dataDir, "volume")
	mkdir(t, masterDir)
	mkdir(t, volumeDir)

	cluster.start(t, ctx, weed, "master",
		"-port", fmt.Sprint(masterPort),
		"-port.grpc", fmt.Sprint(masterGRPCPort),
		"-mdir", masterDir,
		"-ip", "127.0.0.1",
		"-peers", "none",
	)
	cluster.waitForHTTP(t, ctx, cluster.MasterURL+"/cluster/healthz")

	cluster.start(t, ctx, weed, "volume",
		"-port", fmt.Sprint(volumePort),
		"-port.grpc", fmt.Sprint(volumeGRPCPort),
		"-dir", volumeDir,
		"-max", "8",
		"-mserver", fmt.Sprintf("127.0.0.1:%d", masterPort),
		"-ip", "127.0.0.1",
	)
	cluster.waitForHTTP(t, ctx, cluster.VolumeURL+"/status")
	cluster.waitForAssignableVolume(t, ctx)

	t.Cleanup(cluster.Stop)
	return cluster
}

func (c *Cluster) Stop() {
	for i := len(c.cmds) - 1; i >= 0; i-- {
		cmd := c.cmds[i]
		if cmd.Process == nil {
			continue
		}
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
}

func (c *Cluster) start(t *testing.T, ctx context.Context, name string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(ctx, name, args...)
	logPath := filepath.Join(c.dataDir, args[0]+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create log file %s: %v", logPath, err)
	}
	t.Cleanup(func() {
		_ = logFile.Close()
	})
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start weed %s: %v", args[0], err)
	}
	c.cmds = append(c.cmds, cmd)
	c.logs = append(c.logs, logPath)
}

func findWeedBinary(t *testing.T) string {
	t.Helper()

	if path := os.Getenv("WEED_BINARY"); path != "" {
		resolved := resolveExistingPath(t, path)
		assertExecutable(t, resolved)
		return resolved
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve testweed path")
	}
	dir := filepath.Dir(file)
	for {
		candidate := filepath.Join(dir, "weed")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			assertExecutable(t, candidate)
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("weed binary not found; set WEED_BINARY or place ./weed at the project root")
	return ""
}

func resolveExistingPath(t *testing.T, path string) string {
	t.Helper()

	if filepath.IsAbs(path) {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	dir := wd
	for {
		candidate := filepath.Join(dir, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return path
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("weed binary %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("weed binary %s is a directory", path)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("weed binary %s is not executable", path)
	}
}

func freePortPair(t *testing.T) (int, int) {
	t.Helper()

	start := 20000 + int(time.Now().UnixNano()%20000)
	for offset := range 20000 {
		port := 20000 + (start+offset)%20000
		grpcPort := port + 10000
		httpListener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		grpcListener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", grpcPort))
		if err != nil {
			httpListener.Close()
			continue
		}
		httpListener.Close()
		grpcListener.Close()
		return port, grpcPort
	}
	t.Fatal("allocate port pair")
	return 0, 0
}

func mkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func (c *Cluster) waitForHTTP(t *testing.T, ctx context.Context, rawURL string) {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusInternalServerError {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for %s: %v", rawURL, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	c.dumpLogs(t)
	t.Fatalf("timeout waiting for %s", rawURL)
}

func (c *Cluster) waitForAssignableVolume(t *testing.T, ctx context.Context) {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(30 * time.Second)
	assignURL := c.MasterURL + "/dir/assign"
	successes := 0
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, assignURL, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil && resp.StatusCode == http.StatusOK && hasAssignedFID(body) {
				successes++
				if successes == 2 {
					return
				}
			} else {
				successes = 0
			}
		} else {
			successes = 0
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for assignable volume: %v", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	c.dumpLogs(t)
	t.Fatalf("timeout waiting for assignable volume")
}

func hasAssignedFID(body []byte) bool {
	return strings.Contains(string(body), `"fid"`)
}

func (c *Cluster) dumpLogs(t *testing.T) {
	t.Helper()

	for _, logPath := range c.logs {
		content, err := os.ReadFile(logPath)
		if err != nil {
			t.Logf("read %s: %v", logPath, err)
			continue
		}
		t.Logf("%s:\n%s", filepath.Base(logPath), content)
	}
}
