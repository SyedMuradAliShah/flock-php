package daemon

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"flock/internal/api"
	"flock/internal/config"
)

// fakeNpm puts an npm into Flock's bin folder (first on the dev server's PATH) that logs its
// arguments and runs the test binary in mode (see testCommand).
func fakeNpm(t *testing.T, root, mode string) {
	t.Helper()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		script := "@echo npm %*\r\n@\"" + os.Args[0] + "\" " + mode + "\r\n"
		if err := os.WriteFile(filepath.Join(bin, "npm.cmd"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	script := "#!/bin/sh\necho npm \"$@\"\nexec \"" + os.Args[0] + "\" " + mode + "\n"
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// nodeApp is a parked folder with a Node.js app in it.
func nodeApp(t *testing.T, c *config.Config) {
	t.Helper()
	parked := t.TempDir()
	if err := os.Mkdir(filepath.Join(parked, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(parked, "app", "package.json"), `{"scripts": {"dev": "vite"}}`)
	c.Parked = []string{parked}
}

func siteStatus(t *testing.T, client *api.Client, name string) api.SiteStatus {
	t.Helper()
	st, err := client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st.Sites {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no site %s", name)
	return api.SiteStatus{}
}

func getPage(t *testing.T, cfg *config.Config, host string) (int, string, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(cfg.Ports.HTTP)+"/", nil)
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("X-Flock-Page"), string(body)
}

func TestANodeSiteStartsOnItsFirstRequest(t *testing.T) {
	root, cfg := testRoot(t, func(c *config.Config) { nodeApp(t, c) })
	fakeNpm(t, root, "flock-test-server")
	client := startDaemon(t, root)

	s := siteStatus(t, client, "app")
	if s.Type != config.TypeNode || s.NodeServer == nil || s.NodeServer.State != api.NodeSleeping || s.NodeServer.Log != "node-app" {
		t.Fatalf("before the first request: %+v %+v", s, s.NodeServer)
	}
	port := s.NodeServer.Port
	if port < firstNodePort || port >= firstNodePort+nodePorts || s.NodeServer.Command != "npm run dev -- --port "+strconv.Itoa(port) {
		t.Fatalf("port %d, command %q", port, s.NodeServer.Command)
	}

	// The first request starts the dev server and gets the page that waits for it.
	code, page, body := getPage(t, cfg, "app.test")
	if code != http.StatusServiceUnavailable || page != "node-starting" || !strings.Contains(body, "Starting the dev server") {
		t.Fatalf("first request: %d %q\n%s", code, page, body)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		code, _, body = getPage(t, cfg, "app.test")
		if code == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the dev server didn't answer: %d\n%s\nlog:\n%s", code, body, readLog(t, root, "node-app"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	if body != "dev server for app.test" {
		t.Fatalf("got %q", body)
	}
	if s := siteStatus(t, client, "app"); s.NodeServer.State != api.NodeRunning || s.NodeServer.Port != port {
		t.Fatalf("after it answered: %+v", s.NodeServer)
	}
	if log := readLog(t, root, "node-app"); !strings.Contains(log, "npm run dev -- --port "+strconv.Itoa(port)) {
		t.Fatalf("npm didn't get the port:\n%s", log)
	}
}

func TestANodeSiteWhoseDevServerExitsSaysWhy(t *testing.T) {
	root, cfg := testRoot(t, func(c *config.Config) { nodeApp(t, c) })
	fakeNpm(t, root, "flock-test-exit")
	client := startDaemon(t, root)

	getPage(t, cfg, "app.test")
	deadline := time.Now().Add(20 * time.Second)
	for {
		s := siteStatus(t, client, "app")
		if s.NodeServer.State == api.NodeFailed {
			// No node_modules: that's what to fix first.
			if !strings.Contains(s.NodeServer.Problem, "npm install") {
				t.Fatalf("problem: %q", s.NodeServer.Problem)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not failed: %+v", s.NodeServer)
		}
		time.Sleep(100 * time.Millisecond)
	}

	code, page, body := getPage(t, cfg, "app.test")
	if code != http.StatusServiceUnavailable || page != "node-starting" || !strings.Contains(body, "Cannot find module") {
		t.Fatalf("the page doesn't say why: %d %q\n%s", code, page, body)
	}
}

func TestRestartWakesANodeSite(t *testing.T) {
	root, _ := testRoot(t, func(c *config.Config) { nodeApp(t, c) })
	fakeNpm(t, root, "flock-test-server")
	client := startDaemon(t, root)

	if _, err := client.RestartSite(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for siteStatus(t, client, "app").NodeServer.State != api.NodeRunning {
		if time.Now().After(deadline) {
			t.Fatalf("not running: %+v\n%s", siteStatus(t, client, "app").NodeServer, readLog(t, root, "node-app"))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// quickSleep makes dev servers sleep after idle instead of 30 minutes, for the daemons started
// after it.
func quickSleep(t *testing.T, idle time.Duration) {
	t.Helper()
	oldIdle, oldCheck := nodeIdle, nodeSleepCheck
	nodeIdle, nodeSleepCheck = idle, 50*time.Millisecond
	t.Cleanup(func() { nodeIdle, nodeSleepCheck = oldIdle, oldCheck })
}

// waitNode waits until the site's dev server is in state.
func waitNode(t *testing.T, client *api.Client, root, state string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for siteStatus(t, client, "app").NodeServer.State != state {
		if time.Now().After(deadline) {
			t.Fatalf("not %s: %+v\n%s", state, siteStatus(t, client, "app").NodeServer, readLog(t, root, "node-app"))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestAnIdleNodeSiteGoesToSleepAndWakesAgain(t *testing.T) {
	quickSleep(t, 500*time.Millisecond)
	root, cfg := testRoot(t, func(c *config.Config) { nodeApp(t, c) })
	fakeNpm(t, root, "flock-test-server")
	client := startDaemon(t, root)

	getPage(t, cfg, "app.test")
	waitNode(t, client, root, api.NodeRunning)
	port := siteStatus(t, client, "app").NodeServer.Port

	waitNode(t, client, root, api.NodeSleeping)
	if answers("http://localhost:" + strconv.Itoa(port)) {
		t.Fatal("the sleeping dev server still holds its port")
	}

	// The next request wakes it.
	if code, page, _ := getPage(t, cfg, "app.test"); page != "node-starting" {
		t.Fatalf("waking: %d %q", code, page)
	}
	waitNode(t, client, root, api.NodeRunning)
}

func TestAnOpenHotReloadConnectionKeepsANodeSiteAwake(t *testing.T) {
	quickSleep(t, 300*time.Millisecond)
	root, cfg := testRoot(t, func(c *config.Config) { nodeApp(t, c) })
	fakeNpm(t, root, "flock-test-server")
	client := startDaemon(t, root)

	getPage(t, cfg, "app.test")
	waitNode(t, client, root, api.NodeRunning)

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(cfg.Ports.HTTP), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(conn, "GET /@vite/client HTTP/1.1\r\nHost: app.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if resp, err := http.ReadResponse(bufio.NewReader(conn), nil); err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade: %v %+v", err, resp)
	}

	// Several idle periods pass with the tab open.
	time.Sleep(1500 * time.Millisecond)
	if s := siteStatus(t, client, "app").NodeServer.State; s != api.NodeRunning {
		t.Fatalf("slept with a hot-reload connection open: %s", s)
	}

	conn.Close()
	waitNode(t, client, root, api.NodeSleeping)
}

func TestStoppingANodeSitePutsItToSleep(t *testing.T) {
	root, cfg := testRoot(t, func(c *config.Config) { nodeApp(t, c) })
	fakeNpm(t, root, "flock-test-server")
	client := startDaemon(t, root)

	getPage(t, cfg, "app.test")
	waitNode(t, client, root, api.NodeRunning)
	port := siteStatus(t, client, "app").NodeServer.Port

	if _, err := client.StopSite(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	if s := siteStatus(t, client, "app").NodeServer.State; s != api.NodeSleeping || answers("http://localhost:"+strconv.Itoa(port)) {
		t.Fatalf("after the stop: %s", s)
	}
	if _, err := client.StopSite(context.Background(), "missing"); err == nil {
		t.Fatal("stopped a site that doesn't exist")
	}
}

func TestPreferredNodePortsAreStableAndInRange(t *testing.T) {
	for _, name := range []string{"app", "shop", "feature-x.shop"} {
		p := preferredNodePort(name)
		if p != preferredNodePort(name) || p < firstNodePort || p >= firstNodePort+nodePorts {
			t.Fatalf("%s: %d", name, p)
		}
	}
}
