package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"coldmic/internal/domain"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	trimmed := strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: trimmed,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type envelope struct {
	OK     bool              `json:"ok"`
	Error  string            `json:"error,omitempty"`
	Status domain.Status     `json:"status,omitempty"`
	Result domain.StopResult `json:"result,omitempty"`
}

type transcriptEnvelope struct {
	OK       bool              `json:"ok"`
	Error    string            `json:"error,omitempty"`
	Captured time.Time         `json:"captured"`
	Result   domain.StopResult `json:"result"`
}

func (c *Client) Start(ctx context.Context) (domain.Status, error) {
	var env envelope
	if err := c.call(ctx, http.MethodPost, "/v1/session/start", nil, &env); err != nil {
		return domain.Status{}, err
	}
	return env.Status, nil
}

func (c *Client) Stop(ctx context.Context) (domain.Status, domain.StopResult, error) {
	var env envelope
	if err := c.call(ctx, http.MethodPost, "/v1/session/stop", nil, &env); err != nil {
		return domain.Status{}, domain.StopResult{}, err
	}
	return env.Status, env.Result, nil
}

func (c *Client) Status(ctx context.Context) (domain.Status, error) {
	var env envelope
	if err := c.call(ctx, http.MethodGet, "/v1/session/status", nil, &env); err != nil {
		return domain.Status{}, err
	}
	return env.Status, nil
}

func (c *Client) Transcript(ctx context.Context) (time.Time, domain.StopResult, error) {
	var env transcriptEnvelope
	if err := c.call(ctx, http.MethodGet, "/v1/session/transcript/latest", nil, &env); err != nil {
		return time.Time{}, domain.StopResult{}, err
	}
	return env.Captured, env.Result, nil
}

func (c *Client) call(ctx context.Context, method string, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		buf := bytes.NewBuffer(nil)
		if err := json.NewEncoder(buf).Encode(payload); err != nil {
			return err
		}
		body = buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return err
		}
	}

	if resp.StatusCode >= 400 {
		switch v := out.(type) {
		case *envelope:
			return newHTTPError(resp.StatusCode, v.Error)
		case *transcriptEnvelope:
			return newHTTPError(resp.StatusCode, v.Error)
		default:
			return newHTTPError(resp.StatusCode, "request failed")
		}
	}
	return nil
}

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e HTTPError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("request failed with status %d", e.StatusCode)
	}
	return e.Message
}

func newHTTPError(code int, message string) error {
	return HTTPError{StatusCode: code, Message: message}
}
