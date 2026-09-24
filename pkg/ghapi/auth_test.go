package ghapi

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// fakeGh stands in for gh: the token of gh is kept in the file token, and every call is appended to the file calls
const fakeGh = `#!/bin/bash
store="$FAKE_GH_DIR/token"
echo "$*" >> "$FAKE_GH_DIR/calls"
case "$1 $2" in
"auth token")
  [ -s "$store" ] || { echo "no oauth token found" >&2; exit 1; }
  cat "$store" ;;
"auth login")
  if [[ " $* " == *" --with-token "* ]]; then
    [ -n "$FAKE_GH_LOGIN_SLEEP" ] && sleep "$FAKE_GH_LOGIN_SLEEP"
    [ -n "$FAKE_GH_LOGIN_FAIL" ] && { echo "login failed" >&2; exit 1; }
    cat > "$store"
  else
    printf fresh > "$store"
  fi ;;
"auth logout")
  [ -n "$FAKE_GH_LOGOUT_FAIL" ] && { echo "logout failed" >&2; exit 1; }
  : > "$store" ;;
"auth refresh")
  printf elevated > "$store" ;;
"config get")
  echo ssh ;;
esac
`

// setupFakeGh sets up a fake gh logged in with the token previous, and a fake GitHub API that grants the admin:org
// scope to the token elevated only
func setupFakeGh(t *testing.T) (dir, host string) {
	t.Helper()
	dir = t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(fakeGh), 0o755); err != nil {
		t.Fatal(err)
	}
	// gh only sends the token to its host without a port, so the fake API is reached through the host gh.test
	server := httptest.NewTLSServer(http.HandlerFunc(fakeAPI))
	t.Cleanup(server.Close)
	host = "gh.test"
	defaultTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = defaultTransport })

	t.Setenv("GH_PATH", filepath.Join(dir, "gh"))
	t.Setenv("FAKE_GH_DIR", dir)
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("GH_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("GH_HOST", host)
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("FAKE_GH_API", server.Listener.Addr().String())
	redirectToFakeAPI()
	setToken(t, dir, "previous")

	return dir, host
}

// redirectToFakeAPI sends all requests to the fake API
func redirectToFakeAPI() {
	http.DefaultTransport = redirectTransport{
		addr: os.Getenv("FAKE_GH_API"),
		base: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec
	}
}

type redirectTransport struct {
	addr string
	base http.RoundTripper
}

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Host = rt.addr
	return rt.base.RoundTrip(req)
}

func fakeAPI(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "token ")
	scopes := "repo, read:org"
	if token == "elevated" {
		scopes += ", admin:org"
	}
	w.Header().Set("X-OAuth-Scopes", scopes)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"login": "teacher", "id": 1, "token": token})
}

func setToken(t *testing.T, dir, token string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
}

func token(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func calls(t *testing.T, dir string) string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(dir, "calls"))
	return string(data)
}

func markerExists() bool {
	_, err := os.Stat(markerPath())
	return err == nil
}

func testClient(t *testing.T, host string) *api.RESTClient {
	t.Helper()
	client, err := api.NewRESTClient(api.ClientOptions{Host: host, AuthToken: "previous"})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// usedToken returns the token that the client sends to the API
func usedToken(t *testing.T, client *api.RESTClient) string {
	t.Helper()
	var response struct {
		Token string `json:"token"`
	}
	if err := client.Get("user", &response); err != nil {
		t.Fatal(err)
	}
	return response.Token
}

func TestWithScopeRestoresAfterError(t *testing.T) {
	dir, host := setupFakeGh(t)
	client := testClient(t, host)

	var inside string
	returned, err := WithScope(client, "admin:org", func(c *api.RESTClient) error {
		inside = usedToken(t, c)
		if !markerExists() {
			t.Error("no marker while elevated")
		}
		return errors.New("fn failed")
	})

	if err == nil || err.Error() != "fn failed" {
		t.Errorf("err = %v, want fn failed", err)
	}
	if inside != "elevated" {
		t.Errorf("fn used token %q, want elevated", inside)
	}
	if got := token(t, dir); got != "previous" {
		t.Errorf("gh token = %q, want previous", got)
	}
	if got := usedToken(t, returned); got != "previous" {
		t.Errorf("returned client uses %q, want previous", got)
	}
	if markerExists() {
		t.Error("marker left behind")
	}
}

func TestWithScopeRestoresAfterPanic(t *testing.T) {
	dir, host := setupFakeGh(t)

	func() {
		defer func() { _ = recover() }()
		_, _ = WithScope(testClient(t, host), "admin:org", func(c *api.RESTClient) error {
			panic("fn panicked")
		})
	}()

	if got := token(t, dir); got != "previous" {
		t.Errorf("gh token = %q, want previous", got)
	}
	if markerExists() {
		t.Error("marker left behind")
	}
}

func TestWithScopeLogsOutIfRestoreFails(t *testing.T) {
	dir, host := setupFakeGh(t)
	t.Setenv("FAKE_GH_LOGIN_FAIL", "1")

	_, _ = WithScope(testClient(t, host), "admin:org", func(c *api.RESTClient) error { return nil })

	if got := token(t, dir); got != "" {
		t.Errorf("gh token = %q, want logged out", got)
	}
	if markerExists() {
		t.Error("marker left behind")
	}
}

func TestWithScopeNotNested(t *testing.T) {
	dir, host := setupFakeGh(t)

	_, err := WithScope(testClient(t, host), "admin:org", func(c *api.RESTClient) error {
		_, err := WithScope(testClient(t, host), "delete_repo", func(c *api.RESTClient) error { return nil })
		return err
	})

	if err == nil {
		t.Error("nested WithScope succeeded")
	}
	if got := token(t, dir); got != "previous" {
		t.Errorf("gh token = %q, want previous", got)
	}
}

func TestEnsureAuthRecoversWhenDowngradeFailed(t *testing.T) {
	dir, host := setupFakeGh(t)
	t.Setenv("FAKE_GH_LOGIN_FAIL", "1")
	t.Setenv("FAKE_GH_LOGOUT_FAIL", "1")

	_, _ = WithScope(testClient(t, host), "admin:org", func(c *api.RESTClient) error { return nil })
	if got := token(t, dir); got != "elevated" || !markerExists() {
		t.Fatalf("gh token = %q, marker = %v, want elevated with marker", got, markerExists())
	}

	// The next run logs out and asks to log in again
	t.Setenv("FAKE_GH_LOGOUT_FAIL", "")
	if err := EnsureAuth(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(calls(t, dir), "auth logout") {
		t.Error("not logged out")
	}
	if got := token(t, dir); got != "fresh" {
		t.Errorf("gh token = %q, want fresh", got)
	}
	if markerExists() {
		t.Error("marker left behind")
	}
}

func TestEnsureAuthKeepsPreviousToken(t *testing.T) {
	dir, host := setupFakeGh(t)

	// Killed before the scope was added
	err := writeMarker(marker{Host: host, User: "teacher", Scope: "admin:org", Previous: fingerprint("previous"), PID: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureAuth(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(calls(t, dir), "auth logout") {
		t.Error("logged out")
	}
	if got := token(t, dir); got != "previous" {
		t.Errorf("gh token = %q, want previous", got)
	}
	if markerExists() {
		t.Error("marker left behind")
	}
}

// TestWithScopeRestoresOnExit exits in fn like mmc.Fatal does
func TestWithScopeRestoresOnExit(t *testing.T) {
	if os.Getenv("GHAPI_TEST_CHILD") == "exit" {
		redirectToFakeAPI()
		client, _ := api.NewRESTClient(api.ClientOptions{Host: os.Getenv("GH_HOST"), AuthToken: "previous"})
		_, _ = WithScope(client, "admin:org", func(c *api.RESTClient) error {
			RestoreToken()
			os.Exit(1)
			return nil
		})
		os.Exit(0)
	}

	dir, _ := setupFakeGh(t)
	code := runChild(t, dir, "exit", false)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if got := token(t, dir); got != "previous" {
		t.Errorf("gh token = %q, want previous", got)
	}
	if markerExists() {
		t.Error("marker left behind")
	}
}

// TestWithScopeRestoresOnInterrupt presses Ctrl-C in fn and again while gh is downgraded
func TestWithScopeRestoresOnInterrupt(t *testing.T) {
	if os.Getenv("GHAPI_TEST_CHILD") == "interrupt" {
		redirectToFakeAPI()
		client, _ := api.NewRESTClient(api.ClientOptions{Host: os.Getenv("GH_HOST"), AuthToken: "previous"})
		_, _ = WithScope(client, "admin:org", func(c *api.RESTClient) error {
			// Ctrl-C signals the whole foreground process group
			_ = syscall.Kill(0, syscall.SIGINT)
			time.Sleep(10 * time.Second)
			return nil
		})
		os.Exit(0)
	}

	dir, _ := setupFakeGh(t)
	t.Setenv("FAKE_GH_LOGIN_SLEEP", "1")
	code := runChild(t, dir, "interrupt", true)

	if code != 128+int(syscall.SIGINT) {
		t.Errorf("exit code = %d, want %d", code, 128+int(syscall.SIGINT))
	}
	if got := token(t, dir); got != "previous" {
		t.Errorf("gh token = %q, want previous", got)
	}
	if markerExists() {
		t.Error("marker left behind")
	}
}

// runChild runs the test again as a child in its own process group and returns its exit code. With interruptAgain,
// the process group of the child is interrupted again while gh is downgraded.
func runChild(t *testing.T, dir, scenario string, interruptAgain bool) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = append(os.Environ(), "GHAPI_TEST_CHILD="+scenario)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	if interruptAgain {
		deadline := time.Now().Add(10 * time.Second)
		for !strings.Contains(calls(t, dir), "--with-token") && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}

	err := cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return 0
}
