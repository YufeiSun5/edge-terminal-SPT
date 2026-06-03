package edgecontrol

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	ErrDisabled     = errors.New("edge control is disabled")
	ErrMissingToken = errors.New("edge control service token is missing")
)

type Options struct {
	BaseURL         string
	ServiceTokenRef string
	Enabled         bool
	Timeout         time.Duration
}

type Client struct {
	baseURL         string
	serviceTokenRef string
	enabled         bool
	timeout         time.Duration
	httpClient      *http.Client
	getenv          func(string) string
}

type Response struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func NewClient(options Options) *Client {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL:         strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"),
		serviceTokenRef: strings.TrimSpace(options.ServiceTokenRef),
		enabled:         options.Enabled,
		timeout:         timeout,
		httpClient:      &http.Client{Timeout: timeout},
		getenv:          os.Getenv,
	}
}

func (c *Client) ServiceTokenRef() string {
	return c.serviceTokenRef
}

func (c *Client) Forward(ctx context.Context, path string, rawQuery string, body []byte, commandID string) (Response, error) {
	if !c.enabled {
		return Response{}, ErrDisabled
	}
	token := strings.TrimSpace(c.getenv(c.serviceTokenRef))
	if token == "" {
		return Response{}, ErrMissingToken
	}
	target, err := c.targetURL(path, rawQuery)
	if err != nil {
		return Response{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if commandID = strings.TrimSpace(commandID); commandID != "" {
		req.Header.Set("X-Command-ID", commandID)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return Response{}, err
	}
	return Response{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        raw,
	}, nil
}

func (c *Client) ForwardRead(ctx context.Context, path string, rawQuery string) (Response, error) {
	if !c.enabled {
		return Response{}, ErrDisabled
	}
	token := strings.TrimSpace(c.getenv(c.serviceTokenRef))
	if token == "" {
		return Response{}, ErrMissingToken
	}
	target, err := c.targetURL(path, rawQuery)
	if err != nil {
		return Response{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return Response{}, err
	}
	return Response{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        raw,
	}, nil
}

func (c *Client) targetURL(path string, rawQuery string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimPrefix(path, "/")
	base.RawQuery = rawQuery
	return base.String(), nil
}
