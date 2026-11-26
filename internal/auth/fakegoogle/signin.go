package fakegoogle

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SignIn runs the whole browser flow for tests: the app's /auth/google/start,
// the fake account page, and the app's callback. client must have a cookie
// jar and trust the app's TLS certificate. It returns the app's final
// redirect target ("/" or "/welcome", or "/signin?error=...").
func SignIn(client *http.Client, appURL string, id Identity) (string, error) {
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// 1. The app sends us to the provider.
	toProvider, err := location(noFollow.Get(appURL + "/auth/google/start"))
	if err != nil {
		return "", fmt.Errorf("start: %w", err)
	}
	// 2. The "user" picks an account on the provider's page.
	form := toProvider.Query()
	form.Set("sub", id.Sub)
	form.Set("email", id.Email)
	form.Set("name", id.Name)
	if id.EmailVerified {
		form.Set("email_verified", "true")
	}
	authURL := *toProvider
	authURL.RawQuery = ""
	toCallback, err := location(noFollow.PostForm(authURL.String(), form))
	if err != nil {
		return "", fmt.Errorf("authorize: %w", err)
	}
	// 3. The provider sends us back to the app with a code.
	end, err := location(noFollow.Get(toCallback.String()))
	if err != nil {
		return "", fmt.Errorf("callback: %w", err)
	}
	return end.RequestURI(), nil
}

func location(res *http.Response, err error) (*url.URL, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 300 || res.StatusCode > 399 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("status %d, want a redirect: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return res.Location()
}
