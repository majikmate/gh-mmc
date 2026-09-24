package ghapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/config"
)

// ghTimeout limits how long gh may take when downgrading its token, so that a hanging gh cannot prevent it
const ghTimeout = 30 * time.Second

// TokenScopes returns the OAuth scopes of the token of the client. known is false for tokens without OAuth scopes,
// e.g. fine-grained personal access tokens or GitHub App tokens.
func TokenScopes(client *api.RESTClient) (scopes []string, known bool, err error) {
	resp, err := client.Request(http.MethodGet, "user", nil)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.Header.Values("X-OAuth-Scopes") == nil {
		return nil, false, nil
	}
	for _, scope := range strings.Split(resp.Header.Get("X-OAuth-Scopes"), ",") {
		if scope = strings.TrimSpace(scope); scope != "" {
			scopes = append(scopes, scope)
		}
	}

	return scopes, true, nil
}

// WithScope runs fn with a client whose token has the OAuth scope. If the token of gh does not have the scope, gh is
// logged in with a token that has it before running fn, which requires the user to authenticate in the browser. When
// git uses https, git is set up to use the credentials of gh beforehand, so that gh does not ask whether to
// authenticate git with them.
//
// Afterwards, gh is downgraded to its previous token again without any interaction, whatever happens: when adding the
// scope or fn fails or panics, when fn exits with mmc.Fatal, and when the command is interrupted or terminated. If
// restoring the previous token fails, gh is logged out instead. If gh mmc is killed before downgrading, gh is logged
// out on the next run of gh mmc. It returns the client to use afterwards.
func WithScope(client *api.RESTClient, scope string, fn func(client *api.RESTClient) error) (*api.RESTClient, error) {
	scopes, known, err := TokenScopes(client)
	if err != nil {
		return client, fmt.Errorf("failed to get the scopes of the gh token: %v", err)
	}

	// The scopes of tokens without OAuth scopes cannot be changed by gh
	if !known || slices.Contains(scopes, scope) {
		return client, fn(client)
	}

	host, _ := auth.DefaultHost()
	user, err := GetCurrentUser(client)
	if err != nil {
		return client, fmt.Errorf("failed to get the logged in user: %v", err)
	}

	e, err := elevate(host, user.Login, scope)
	if err != nil {
		return client, err
	}
	defer e.restore()

	// Let git use the credentials of gh, which answers yes to the question of gh whether to authenticate git with the
	// credentials of gh when refreshing the token over https
	if protocol, _, err := gh.Exec("config", "get", "git_protocol", "--host", host); err == nil && strings.TrimSpace(protocol.String()) == "https" {
		if _, stderr, err := gh.Exec("auth", "setup-git", "--hostname", host); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set up git to use the credentials of gh: %v: %s\n", err, strings.TrimSpace(stderr.String()))
		}
	}

	fmt.Printf("\nAdding the %s scope to the gh token, which requires to authenticate in the browser.\n", scope)
	err = gh.ExecInteractive(context.Background(), "auth", "refresh", "--hostname", host, "--scopes", scope)
	if err != nil {
		return client, fmt.Errorf("failed to add the %s scope to the gh token: %v", scope, err)
	}

	token, err := ghToken(host, user.Login)
	if err != nil {
		return client, fmt.Errorf("failed to get the gh token: %v", err)
	}

	// The elevated client refuses requests once the command is interrupted, so that fn does not go on meanwhile
	elevated, err := api.NewRESTClient(api.ClientOptions{Host: host, AuthToken: token, Transport: e})
	if err != nil {
		return client, fmt.Errorf("failed to create gh client: %v", err)
	}

	return client, fn(elevated)
}

// RestoreToken downgrades gh to its previous token while WithScope runs, which must be done before exiting the
// command. It does nothing otherwise.
func RestoreToken() {
	elevationMu.Lock()
	e := running
	elevationMu.Unlock()

	if e != nil {
		e.restore()
	}
}

// EnsureAuth prepares gh for a command. If a run of gh mmc was killed before downgrading gh, gh is logged out, since
// its previous token is not known anymore. If gh is not logged in, the user is asked to log in in the browser.
func EnsureAuth() error {
	if err := recoverElevation(); err != nil {
		return err
	}

	host, _ := auth.DefaultHost()
	if _, err := ghToken(host, ""); err == nil {
		return nil
	}

	fmt.Printf("gh is not logged in to %s, which requires to authenticate in the browser.\n", host)
	err := gh.ExecInteractive(context.Background(), "auth", "login", "--hostname", host, "--web")
	if err != nil {
		return fmt.Errorf("failed to log in to %s with gh: %v", host, err)
	}

	return nil
}

// elevation is gh being logged in with a token that has an additional scope while WithScope runs. The previous token
// of gh is only kept in memory to restore it afterwards.
type elevation struct {
	host        string
	user        string
	scope       string
	previous    string
	interrupted atomic.Bool
	once        sync.Once
	stop        func()
}

// running is the elevation while WithScope runs
var (
	elevationMu sync.Mutex
	running     *elevation
)

// elevate prepares to log gh in with a token that has the scope, so that gh is downgraded to its current token again
// whatever happens
func elevate(host, user, scope string) (*elevation, error) {
	elevationMu.Lock()
	defer elevationMu.Unlock()

	if running != nil {
		return nil, fmt.Errorf("cannot add the %s scope to the gh token while the %s scope is added", scope, running.scope)
	}
	// Another run would restore the token with its scope as the previous token
	if m, err := readMarker(); err == nil && processAlive(m.PID) {
		return nil, fmt.Errorf("cannot add the %s scope to the gh token while another run of gh mmc (pid %d) added the %s scope: try again afterwards", scope, m.PID, m.Scope)
	}

	previous, err := ghToken(host, user)
	if err != nil {
		return nil, fmt.Errorf("failed to get the gh token: %v", err)
	}

	// The marker is written before the scope is added, so that a killed run can be detected on the next run
	err = writeMarker(marker{Host: host, User: user, Scope: scope, Previous: fingerprint(previous), PID: os.Getpid()})
	if err != nil {
		return nil, fmt.Errorf("failed to record adding the %s scope to the gh token: %v", scope, err)
	}

	e := &elevation{host: host, user: user, scope: scope, previous: previous}
	e.stop = e.watchSignals()
	running = e

	return e, nil
}

// restore downgrades gh to its previous token, or logs gh out if that fails. It is done only once, needs no
// interaction and cannot be interrupted.
func (e *elevation) restore() {
	e.once.Do(func() {
		// Signals are ignored until gh is downgraded
		defer e.stop()
		defer func() {
			elevationMu.Lock()
			running = nil
			elevationMu.Unlock()
		}()

		fmt.Printf("\nRemoving the %s scope from the gh token again.\n", e.scope)
		err := downgrade(e.host, e.user, e.previous)
		if err != nil {
			// The marker is kept, so that gh is logged out on the next run
			fmt.Fprintf(os.Stderr, "Failed to remove the %s scope from the gh token: %v\n", e.scope, err)
			return
		}
		if err := removeMarker(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", markerPath(), err)
		}
	})
}

// watchSignals downgrades gh and exits when the command is interrupted or terminated, or its output is closed. It
// returns a function that stops watching.
func (e *elevation) watchSignals() func() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGPIPE)
	done := make(chan struct{})

	go func() {
		select {
		case sig := <-signals:
			e.interrupted.Store(true)
			fmt.Fprintf(os.Stderr, "\nInterrupted by %v.\n", sig)
			e.restore()
			code := 1
			if s, ok := sig.(syscall.Signal); ok {
				code = 128 + int(s)
			}
			os.Exit(code)
		case <-done:
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			signal.Stop(signals)
			close(done)
		})
	}
}

// RoundTrip sends the requests of the elevated client until the command is interrupted
func (e *elevation) RoundTrip(req *http.Request) (*http.Response, error) {
	if e.interrupted.Load() {
		return nil, errors.New("interrupted")
	}
	return http.DefaultTransport.RoundTrip(req)
}

// downgrade logs the user in to gh with the previous token again, or logs the user out of gh if that fails
func downgrade(host, user, previous string) error {
	_, err := ghDetached(previous, "auth", "login", "--hostname", host, "--with-token")
	if err == nil {
		var token string
		token, err = ghToken(host, user)
		if err == nil && token != previous {
			err = errors.New("gh is logged in with another token")
		}
	}
	if err == nil {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Failed to restore the previous gh token: %v\n", err)
	fmt.Fprintf(os.Stderr, "Logging out of gh instead, the next gh mmc command asks to log in again.\n")
	return logout(host, user)
}

// logout logs the user out of gh
func logout(host, user string) error {
	_, err := ghDetached("", "auth", "logout", "--hostname", host, "--user", user)
	if err != nil {
		// Not being logged in anymore is fine as well
		if _, tokenErr := ghToken(host, user); tokenErr != nil {
			return nil
		}
		return fmt.Errorf("failed to log out of gh: run `gh auth logout --hostname %s --user %s`: %v", host, user, err)
	}
	return nil
}

// recoverElevation logs gh out if a run of gh mmc was killed before downgrading gh
func recoverElevation() error {
	m, err := readMarker()
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	// A marker that cannot be read was written incompletely, i.e. before the scope was added
	if err != nil {
		return removeMarker()
	}

	// Another run of gh mmc is still working with the elevated token
	if processAlive(m.PID) {
		return nil
	}

	token, err := ghToken(m.Host, m.User)
	if err != nil {
		// The user is not logged in anymore
		return removeMarker()
	}
	if fingerprint(token) == m.Previous {
		// The run was killed before the scope was added or after the previous token was restored
		return removeMarker()
	}
	if client, err := api.NewRESTClient(api.ClientOptions{Host: m.Host, AuthToken: token}); err == nil {
		if scopes, known, err := TokenScopes(client); err == nil && known && !slices.Contains(scopes, m.Scope) {
			// The scope was removed from the token in the meantime
			return removeMarker()
		}
	}

	fmt.Printf("A previous run of gh mmc ended before removing the %s scope from the gh token again: logging out of gh.\n", m.Scope)
	if err := logout(m.Host, m.User); err != nil {
		return err
	}
	return removeMarker()
}

// marker records an elevation on disk, so that gh can be logged out on the next run if gh mmc is killed before
// downgrading gh. Instead of the previous token, it only contains its fingerprint.
type marker struct {
	Host     string `json:"host"`
	User     string `json:"user"`
	Scope    string `json:"scope"`
	Previous string `json:"previous"`
	PID      int    `json:"pid"`
}

// markerPath returns the path of the file of the marker in the state folder of gh
func markerPath() string {
	return filepath.Join(config.StateDir(), "gh-mmc", "elevation.json")
}

func writeMarker(m marker) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(markerPath()), 0o700); err != nil {
		return err
	}
	return os.WriteFile(markerPath(), data, 0o600)
}

func readMarker() (marker, error) {
	var m marker
	data, err := os.ReadFile(markerPath())
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	if m.Host == "" || m.User == "" || m.Scope == "" || m.Previous == "" {
		return m, errors.New("incomplete marker")
	}
	return m, nil
}

func removeMarker() error {
	err := os.Remove(markerPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// fingerprint returns a hash of the token that does not reveal the token
func fingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// processAlive checks if the process with the pid is running
func processAlive(pid int) bool {
	if pid <= 0 || pid == os.Getpid() {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// ghToken returns the token of the user for the host that gh is logged in with, or of the active user if user is empty.
// It asks gh each time, since the token changes while gh mmc runs.
func ghToken(host, user string) (string, error) {
	args := []string{"auth", "token", "--hostname", host}
	if user != "" {
		args = append(args, "--user", user)
	}
	stdout, err := ghDetached("", args...)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(stdout)
	if token == "" {
		return "", fmt.Errorf("gh is not logged in to %s", host)
	}
	return token, nil
}

// ghDetached runs gh with the input on stdin and returns its stdout. gh runs in its own process group, so that Ctrl-C
// does not interrupt it, and is killed if it does not finish within ghTimeout.
func ghDetached(input string, args ...string) (string, error) {
	ghExe, err := gh.Path()
	if err != nil {
		return "", fmt.Errorf("failed to find gh: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ghExe, args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%v: %s", err, msg)
		}
		return "", err
	}

	return stdout.String(), nil
}
