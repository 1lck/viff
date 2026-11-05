package updater

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultBaseURL is the public GitHub API root.
	defaultBaseURL = "https://api.github.com"
	// defaultHTTPTimeout bounds one request/response round trip, including the
	// body read for metadata. Downloads use the same client.
	defaultHTTPTimeout = 30 * time.Second
	// maxRedirects is the redirect budget for release downloads. GitHub serves
	// assets from a redirect to an object-storage host.
	maxRedirects = 10
	// userAgentPrefix identifies the running build to the API.
	userAgentPrefix = "Viff/"
	// githubAccept selects the current GitHub REST media type.
	githubAccept = "application/vnd.github+json"
)

// now returns the client clock, defaulting to time.Now.
func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// goos returns the target operating system, defaulting to the build's.
func (c *Client) goos() string {
	if c.GOOS != "" {
		return c.GOOS
	}
	return runtime.GOOS
}

// goarch returns the target architecture, defaulting to the build's.
func (c *Client) goarch() string {
	if c.GOARCH != "" {
		return c.GOARCH
	}
	return runtime.GOARCH
}

// baseURL returns the API root without a trailing slash.
func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return defaultBaseURL
}

// repo returns "owner/name", defaulting to DefaultRepo.
func (c *Client) repo() string {
	if c.Repo != "" {
		return strings.Trim(c.Repo, "/")
	}
	return DefaultRepo
}

// httpClient returns a per-call copy of the configured client with a redirect
// policy: bounded redirects that never leave HTTPS. The copy keeps a caller's
// custom transport and timeout without mutating the shared client.
func (c *Client) httpClient() *http.Client {
	var client *http.Client
	if c.HTTP != nil {
		copied := *c.HTTP
		client = &copied
	} else {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("updater: stopped after %d redirects", maxRedirects)
		}
		if err := requireHTTPS(req.URL); err != nil {
			return err
		}
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
	return client
}

// requireHTTPS rejects cleartext redirect targets. Loopback HTTP is allowed so
// hermetic tests can use httptest servers.
func requireHTTPS(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("updater: redirect without a URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("updater: refusing insecure redirect to %s", u.Redacted())
}

// isLoopbackHost reports whether host names the local machine.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// newRequest builds a request with the headers every GitHub call needs.
func (c *Client) newRequest(ctx context.Context, rawURL, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("updater: build request for %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", userAgentPrefix+c.Current)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return req, nil
}

// get performs one GET and returns a successful response. Callers own the body.
// It enforces HTTPS everywhere except loopback, turns rate limiting into a
// readable error, and reports non-200 status codes.
func (c *Client) get(ctx context.Context, rawURL, accept string) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("updater: parse URL %s: %w", rawURL, err)
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return nil, fmt.Errorf("updater: refusing insecure URL %s", u.Redacted())
	}
	req, err := c.newRequest(ctx, rawURL, accept)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("updater: request %s: %w", u.Redacted(), ctxErr)
		}
		return nil, fmt.Errorf("updater: request %s: %w", u.Redacted(), err)
	}
	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, rateLimitError(resp, u)
	case resp.StatusCode != http.StatusOK:
		snippet := readBodySnippet(resp.Body)
		return nil, fmt.Errorf("updater: %s returned HTTP %d %s%s", u.Redacted(), resp.StatusCode, http.StatusText(resp.StatusCode), snippet)
	}
	return resp, nil
}

// rateLimitError converts a 403/429 response into an actionable message.
func rateLimitError(resp *http.Response, u *url.URL) error {
	defer resp.Body.Close()
	body := readBodySnippet(resp.Body)
	if resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.StatusCode == http.StatusTooManyRequests {
		when := "later"
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if seconds, err := strconv.ParseInt(reset, 10, 64); err == nil {
				when = time.Unix(seconds, 0).UTC().Format(time.RFC3339)
			}
		}
		return fmt.Errorf("updater: GitHub API rate limit exceeded for %s; retry after %s%s", u.Redacted(), when, body)
	}
	return fmt.Errorf("updater: GitHub API denied access to %s (HTTP %d)%s", u.Redacted(), resp.StatusCode, body)
}

// readBodySnippet consumes and closes up to 200 bytes of an error body for
// context. It always closes the body and never fails.
func readBodySnippet(body io.ReadCloser) string {
	if body == nil {
		return ""
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, 200))
	if err != nil || len(data) == 0 {
		return ""
	}
	text := strings.Join(strings.Fields(string(data)), " ")
	if text == "" {
		return ""
	}
	return " (" + text + ")"
}

// readAllLimited reads at most limit bytes and reports an error when the body
// is larger, so a hostile or broken response cannot exhaust memory.
func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, nil
}
