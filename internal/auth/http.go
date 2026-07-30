package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func postForm(ctx context.Context, client *http.Client, endpoint string, form url.Values, out any) error {
	if client == nil {
		client = HTTPClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s: %s", resp.Status, errorExcerpt(resp.Body))
	}
	body, err := readBounded(resp, "oauth", maxTokenResponseBytes)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
