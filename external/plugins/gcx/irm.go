package gcx

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/external/plugins/internal/httpx"
)

const (
	defaultRPCTimeout   = 20 * time.Second
	maxRPCResponseBytes = 4 << 20
)

var (
	rpcSep = "."

	methodQueryIncidentPreviews = rpcMethod("IncidentsService", "QueryIncidentPreviews")
	methodCreateIncident        = rpcMethod("IncidentsService", "CreateIncident")
	methodAddActivity           = rpcMethod("ActivityService", "AddActivity")
)

func rpcMethod(service, method string) string { return "/" + service + rpcSep + method }

// Client is one authenticated IRM RPC channel against one stack.
type Client struct {
	BaseURL string
	Stack   string
	Token   string
	HTTP    *http.Client
	Timeout time.Duration
}

// NewClient builds an IRM client for a stack host and service-account token.
func NewClient(stack, token string) (*Client, error) {
	host, err := normalizeStack(stack)
	if err != nil {
		return nil, err
	}
	base, err := IRMBaseURL(host)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, errx.New("gcx: empty Grafana Cloud token").
			WithHint("run `mino login gcx`")
	}
	return &Client{BaseURL: base, Stack: host, Token: token}, nil
}

func (c *Client) rpc(ctx context.Context, method string, in, out any) error {
	what := "gcx irm " + strings.TrimPrefix(method, "/")

	payload, err := json.Marshal(in)
	if err != nil {
		return errx.Wrapf(err, "%s: encoding request", what)
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultRPCTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+method, bytes.NewReader(payload))
	if err != nil {
		return errx.Wrapf(err, "%s: building request", what)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := c.HTTP
	if client == nil {
		client = httpx.Client()
	}
	resp, err := client.Do(req)
	if err != nil {
		return errx.Wrapf(err, "%s: request failed", what)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		e := errx.Newf("%s: %s: %s", what, resp.Status, httpx.ErrorExcerpt(resp.Body))
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			e = e.WithHint("the sealed gcx token is missing, expired, or lacks IRM access; run `mino login gcx`")
		case http.StatusNotFound:
			e = e.WithHint("the IRM RPC path is unverified (see the rpcSep block in irm.go and SPIKE.md §6); confirm the method name against the stack")
		}
		return e
	}

	raw, err := httpx.ReadBounded(resp, what, maxRPCResponseBytes)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errx.Wrapf(err, "%s: decoding response", what)
	}
	return nil
}

// IncidentQuery selects which incidents to list.
type IncidentQuery struct {
	Status        string
	Limit         int
	IncludeDrills bool
}

// QueryIncidents lists incidents on the stack, newest first.
func (c *Client) QueryIncidents(ctx context.Context, q IncidentQuery) ([]incident, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	var terms []string
	if q.Status != "" && q.Status != StatusAll {
		terms = append(terms, "status:"+q.Status)
	}
	if !q.IncludeDrills {
		terms = append(terms, "isdrill:false")
	}

	body := map[string]any{"query": map[string]any{
		"limit":          limit,
		"orderDirection": "DESC",
		"orderField":     "createdTime",
		"queryString":    strings.Join(terms, " "),
	}}

	var env incidentsEnvelope
	if err := c.rpc(ctx, methodQueryIncidentPreviews, body, &env); err != nil {
		return nil, err
	}
	return normalizeAll(env.list(), c.Stack), nil
}

// NewIncident describes an incident to declare.
type NewIncident struct {
	Title    string
	Severity string
	Status   string
	Summary  string
	Labels   []string
	IsDrill  bool
}

// CreateIncident declares an incident on the stack.
func (c *Client) CreateIncident(ctx context.Context, in NewIncident) (incident, error) {
	body := map[string]any{
		"title":    in.Title,
		"severity": in.Severity,
		"status":   in.Status,
		"summary":  in.Summary,
		"labels":   in.Labels,
		"isDrill":  in.IsDrill,
	}
	var env incidentEnvelope
	if err := c.rpc(ctx, methodCreateIncident, body, &env); err != nil {
		return incident{}, err
	}
	return env.Incident.normalize(c.Stack), nil
}

// AddActivity posts an activity item to an incident.
func (c *Client) AddActivity(ctx context.Context, incidentID, body, kind string) error {
	payload := map[string]any{
		"incidentID":   incidentID,
		"activityKind": kind,
		"body":         body,
	}
	return c.rpc(ctx, methodAddActivity, payload, nil)
}
