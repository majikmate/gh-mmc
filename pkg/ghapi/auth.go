package ghapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
)

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

// WithScope runs fn with a client whose token has the OAuth scope. If the token of gh does not have the scope, the
// scope is added to it before running fn and removed from it again afterwards, both requiring the user to
// authenticate in the browser. It returns the client to use afterwards.
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
	fmt.Printf("\nAdding the %s scope to the gh token, which requires to authenticate in the browser.\n", scope)
	err = gh.ExecInteractive(context.Background(), "auth", "refresh", "--hostname", host, "--scopes", scope)
	if err != nil {
		return client, fmt.Errorf("failed to add the %s scope to the gh token: %v", scope, err)
	}

	elevated, err := api.DefaultRESTClient()
	if err != nil {
		err = fmt.Errorf("failed to create gh client: %v", err)
	} else {
		err = fn(elevated)
	}

	fmt.Printf("\nRemoving the %s scope from the gh token again, which requires to authenticate in the browser.\n", scope)
	dropErr := gh.ExecInteractive(context.Background(), "auth", "refresh", "--hostname", host, "--remove-scopes", scope)
	if dropErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove the %s scope from the gh token: run `gh auth refresh --hostname %s --remove-scopes %s`: %v\n", scope, host, scope, dropErr)
	}

	dropped, clientErr := api.DefaultRESTClient()
	if clientErr != nil {
		return client, err
	}

	return dropped, err
}
