package argocd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/external/plugins/internal/httpx"
)

const maxResponseBytes = 8 << 20

type Client struct {
	HTTP   *http.Client
	cfg    Config
	tokens TokenLookup

	once sync.Once
	tls  *http.Client
	err  error
}

func NewClient(cfg Config, tokens TokenLookup) *Client {
	return &Client{cfg: cfg, tokens: tokens}
}

func (c *Client) client() (*http.Client, error) {
	if c.HTTP != nil {
		return c.HTTP, nil
	}
	if strings.TrimSpace(c.cfg.CAFile) == "" {
		return httpx.Client(), nil
	}
	c.once.Do(func() {
		pem, err := os.ReadFile(c.cfg.CAFile)
		if err != nil {
			c.err = errx.Wrapf(err, "argocd: reading ca_file %q", c.cfg.CAFile).
				WithHint("plugins.argocd.ca_file must point at a readable PEM bundle")
			return
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			c.err = errx.Newf("argocd: ca_file %q contains no certificates", c.cfg.CAFile).
				WithHint("export the ArgoCD server's CA chain in PEM form")
			return
		}
		base, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			c.err = errx.New("argocd: cannot clone the default HTTP transport for ca_file")
			return
		}
		t := base.Clone()
		t.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		c.tls = &http.Client{Transport: t, Timeout: httpx.Timeout}
	})
	if c.err != nil {
		return nil, c.err
	}
	return c.tls, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, what string, out any) (http.Header, error) {
	client, err := c.client()
	if err != nil {
		return nil, err
	}
	token, err := resolveToken(ctx, c.tokens, c.cfg.TokenEnv)
	shared.noteAuth(err == nil)
	if err != nil {
		return nil, err
	}

	endpoint := c.cfg.ServerURL + "/api/v1" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errx.Wrapf(err, "argocd: building the %s request", what)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, errx.Wrapf(err, "argocd: %s", what).
			WithHint("check that plugins.argocd.server_url points at a reachable ArgoCD server")
	}
	defer resp.Body.Close()

	body, readErr := httpx.ReadBounded(resp, "argocd "+what, maxResponseBytes)
	if resp.StatusCode >= 400 {
		return resp.Header, c.statusError(resp, body, what)
	}
	if readErr != nil {
		return resp.Header, readErr
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return resp.Header, errx.Wrapf(err, "argocd: decoding the %s response", what).
				WithHint("the server returned %s; check that server_url is the ArgoCD API host and not a proxy",
					errx.ExcerptBytes(body))
		}
	}
	return resp.Header, nil
}

func (c *Client) statusError(resp *http.Response, body []byte, what string) error {
	detail := argoMessage(body)
	host := serverHost(c.cfg.ServerURL)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return errx.Newf("argocd: %s: unauthorized (%s)", what, detail).
			WithHint("the token for %s is missing, expired, or from another server; mint a new one with "+
				"`argocd account generate-token --account mino`", host)
	case http.StatusForbidden:
		return errx.Newf("argocd: %s: forbidden (%s)", what, detail).
			WithHint("the ArgoCD RBAC policy denied this call; the message above names the exact " +
				"resource/action/object, grant it with e.g. `p, role:mino, applications, get, */*, allow`")
	case http.StatusNotFound:
		return errx.Newf("argocd: %s: not found (%s)", what, detail).
			WithHint("check the app name and plugins.argocd.app_namespace; apps outside the default " +
				"`argocd` namespace need appNamespace set")
	case http.StatusTooManyRequests:
		return errx.Newf("argocd: %s: rate limited (%s)", what, detail).
			WithHint("the stream honours Retry-After; raise the `interval` query param to poll less often")
	}
	if resp.StatusCode >= 500 {
		return errx.Newf("argocd: %s: server error %s (%s)", what, resp.Status, detail).
			WithHint("the ArgoCD API server is unhealthy or restarting; this usually clears on its own")
	}
	return errx.Newf("argocd: %s: %s (%s)", what, resp.Status, detail)
}

func argoMessage(body []byte) string {
	var e argoError
	if err := json.Unmarshal(body, &e); err == nil {
		if msg := strings.TrimSpace(e.Error); msg != "" {
			return msg
		}
		if msg := strings.TrimSpace(e.Message); msg != "" {
			return msg
		}
	}
	if excerpt := errx.ExcerptBytes(body); excerpt != "" {
		return excerpt
	}
	return "no detail"
}

func (c *Client) listQuery() url.Values {
	q := url.Values{}
	if c.cfg.App != "" {
		q.Set("name", c.cfg.App)
	}
	for _, p := range c.cfg.Projects {
		q.Add("project", p)
	}
	if c.cfg.Selector != "" {
		q.Set("selector", c.cfg.Selector)
	}
	if c.cfg.AppNamespace != "" {
		q.Set("appNamespace", c.cfg.AppNamespace)
	}
	return q
}

func (c *Client) Applications(ctx context.Context) (applicationList, http.Header, error) {
	var list applicationList
	hdr, err := c.get(ctx, "/applications", c.listQuery(), "listing applications", &list)
	return list, hdr, err
}

func (c *Client) Application(ctx context.Context, name, appNamespace string) (application, error) {
	var app application
	_, err := c.get(ctx, "/applications/"+url.PathEscape(name), appNamespaceQuery(appNamespace),
		"reading application "+name, &app)
	return app, err
}

func (c *Client) ResourceTree(ctx context.Context, name, appNamespace string) (resourceTree, error) {
	var tree resourceTree
	_, err := c.get(ctx, "/applications/"+url.PathEscape(name)+"/resource-tree", appNamespaceQuery(appNamespace),
		"reading the resource tree for "+name, &tree)
	return tree, err
}

func (c *Client) RevisionMetadata(ctx context.Context, name, appNamespace, revision string) (revisionMetadata, error) {
	var meta revisionMetadata
	path := "/applications/" + url.PathEscape(name) + "/revisions/" + url.PathEscape(revision) + "/metadata"
	_, err := c.get(ctx, path, appNamespaceQuery(appNamespace), "reading revision metadata for "+name, &meta)
	return meta, err
}

func appNamespaceQuery(appNamespace string) url.Values {
	if strings.TrimSpace(appNamespace) == "" {
		return nil
	}
	return url.Values{"appNamespace": []string{appNamespace}}
}

func retryAfterFrom(hdr http.Header) time.Duration {
	if hdr == nil {
		return 0
	}
	d, ok := httpx.RetryAfter(hdr, time.Now())
	if !ok {
		return 0
	}
	return d
}
