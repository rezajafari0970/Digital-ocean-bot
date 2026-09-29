package sanaei

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

var ErrAPIRequest = errors.New("x-ui api request failed")

type Credentials struct {
	Username string
	Password string
}
type APIClient struct {
	BaseURL     string
	HTTP        *http.Client
	Credentials Credentials
	CSRF        string
}

func NewAPIClient(baseURL string, credentials Credentials, transport http.RoundTripper) (*APIClient, error) {
	if baseURL == "" || credentials.Username == "" || credentials.Password == "" {
		return nil, ErrAPIRequest
	}
	jar, _ := cookiejar.New(nil)
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &APIClient{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Transport: transport, Jar: jar}, Credentials: credentials}, nil
}

func (c *APIClient) Login(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/csrf-token", nil)
	if err != nil {
		return fmt.Errorf("%w: csrf request: %v", ErrAPIRequest, err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: csrf transport: %w", ErrAPIRequest, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: csrf status %d", ErrAPIRequest, resp.StatusCode)
	}
	var token struct {
		Success bool   `json:"success"`
		Obj     string `json:"obj"`
	}
	if json.NewDecoder(resp.Body).Decode(&token) != nil || !token.Success || token.Obj == "" {
		return ErrAPIRequest
	}
	c.CSRF = token.Obj
	payload, _ := json.Marshal(map[string]string{"username": c.Credentials.Username, "password": c.Credentials.Password})
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/login", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w: login request: %v", ErrAPIRequest, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.CSRF)
	resp, err = c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: login transport: %w", ErrAPIRequest, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrAPIRequest, resp.StatusCode)
	}
	return nil
}

func (c *APIClient) json(ctx context.Context, method, path string, body url.Values, out any) error {
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return ErrAPIRequest
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.CSRF != "" {
		req.Header.Set("X-CSRF-Token", c.CSRF)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: api transport: %w", ErrAPIRequest, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrAPIRequest, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return ErrAPIRequest
		}
	}
	return nil
}
