// Package client is a small HTTP client for the Shieldome REST API.
//
// Every request carries the account's API key in the X-Shieldome-Key header.
// A 404 is returned as ErrNotFound so resources can drop themselves from
// state when they were deleted outside Terraform.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const APIKeyHeader = "X-Shieldome-Key"

var ErrNotFound = errors.New("not found")

// APIError is a non-2xx answer with the server's error message and code.
type APIError struct {
	Status  int
	Message string
	Code    string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("shieldome API %d (%s): %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("shieldome API %d: %s", e.Status, e.Message)
}

type Client struct {
	BaseURL   string
	APIKey    string
	HTTP      *http.Client
	UserAgent string
}

func New(baseURL, apiKey, version string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		APIKey:    apiKey,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		UserAgent: "terraform-provider-shieldome/" + version,
	}
}

// Do sends a JSON request and decodes a JSON answer into out (may be nil).
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set(APIKeyHeader, c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(data))
			if len(e.Error) > 300 {
				e.Error = e.Error[:300]
			}
		}
		return &APIError{Status: resp.StatusCode, Message: e.Error, Code: e.Code}
	}
	// A login page instead of JSON means the key was not accepted.
	if ct := resp.Header.Get("Content-Type"); out != nil && !strings.Contains(ct, "json") {
		return &APIError{Status: resp.StatusCode, Message: "the API did not answer with JSON - check api_key and base_url"}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// ── Sites (authorized domains) ───────────────────────────────────────────────

type VerifyRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Site struct {
	ID                 string        `json:"id"`
	Domain             string        `json:"domain"`
	IsVerified         bool          `json:"is_verified"`
	VerificationActive bool          `json:"verification_active"`
	VerifyMethod       *string       `json:"verify_method"`
	VerifiedAt         *string       `json:"verified_at"`
	VerifyExpiresAt    *string       `json:"verify_expires_at"`
	VerifyRecord       *VerifyRecord `json:"verify_record"`
}

func (c *Client) CreateSite(ctx context.Context, domain string) (*Site, error) {
	var s Site
	err := c.Do(ctx, http.MethodPost, "/api/me/allowed-sites", map[string]string{"domain": domain}, &s)
	return &s, err
}

func (c *Client) GetSite(ctx context.Context, id string) (*Site, error) {
	var s Site
	err := c.Do(ctx, http.MethodGet, "/api/me/allowed-sites/"+url.PathEscape(id), nil, &s)
	return &s, err
}

func (c *Client) ListSites(ctx context.Context) ([]Site, error) {
	var out []Site
	err := c.Do(ctx, http.MethodGet, "/api/me/allowed-sites", nil, &out)
	return out, err
}

type VerifyInit struct {
	AlreadyVerified bool          `json:"already_verified"`
	Token           string        `json:"token"`
	VerifyRecord    *VerifyRecord `json:"verify_record"`
}

func (c *Client) InitSiteVerification(ctx context.Context, id string) (*VerifyInit, error) {
	var v VerifyInit
	err := c.Do(ctx, http.MethodPost, "/api/me/allowed-sites/"+url.PathEscape(id)+"/verify/init",
		map[string]string{"method": "dns_txt"}, &v)
	return &v, err
}

type VerifyCheck struct {
	Verified   bool   `json:"verified"`
	VerifiedAt string `json:"verified_at"`
	Message    string `json:"message"`
}

func (c *Client) CheckSiteVerification(ctx context.Context, id string) (*VerifyCheck, error) {
	var v VerifyCheck
	err := c.Do(ctx, http.MethodPost, "/api/me/allowed-sites/"+url.PathEscape(id)+"/verify/check", map[string]string{}, &v)
	return &v, err
}

// ── Scheduled scans ──────────────────────────────────────────────────────────

type ScheduledScan struct {
	ID            string  `json:"id,omitempty"`
	TargetURL     string  `json:"target_url"`
	ScanType      string  `json:"scan_type"`
	IntervalHours int64   `json:"interval_hours"`
	Enabled       bool    `json:"enabled"`
	NotifyEmail   string  `json:"notify_email"`
	WebhookURL    string  `json:"webhook_url"`
	ExcludedPaths string  `json:"excluded_paths"`
	NextRun       *string `json:"next_run,omitempty"`
	LastRun       *string `json:"last_run,omitempty"`
}

func (c *Client) CreateScheduledScan(ctx context.Context, in ScheduledScan) (*ScheduledScan, error) {
	var out ScheduledScan
	err := c.Do(ctx, http.MethodPost, "/api/scheduled-scans", in, &out)
	return &out, err
}

func (c *Client) GetScheduledScan(ctx context.Context, id string) (*ScheduledScan, error) {
	var out ScheduledScan
	err := c.Do(ctx, http.MethodGet, "/api/scheduled-scans/"+url.PathEscape(id), nil, &out)
	return &out, err
}

func (c *Client) UpdateScheduledScan(ctx context.Context, id string, patch map[string]any) (*ScheduledScan, error) {
	var out ScheduledScan
	err := c.Do(ctx, http.MethodPatch, "/api/scheduled-scans/"+url.PathEscape(id), patch, &out)
	return &out, err
}

func (c *Client) DeleteScheduledScan(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/scheduled-scans/"+url.PathEscape(id), nil, nil)
}

// ── Scan profiles ────────────────────────────────────────────────────────────

type ScanProfile struct {
	ID              string            `json:"id,omitempty"`
	Name            string            `json:"name"`
	TargetURL       string            `json:"target_url"`
	ScanType        string            `json:"scan_type"`
	ExcludedPaths   string            `json:"excluded_paths"`
	AuthCookie      string            `json:"auth_cookie"`
	AuthHeader      string            `json:"auth_header"`
	CustomHeaders   map[string]string `json:"custom_headers"`
	UsePlaywright   bool              `json:"use_playwright"`
	PlatformProfile *string           `json:"platform_profile"`
}

func (c *Client) CreateScanProfile(ctx context.Context, in ScanProfile) (*ScanProfile, error) {
	var out ScanProfile
	err := c.Do(ctx, http.MethodPost, "/api/profiles", in, &out)
	return &out, err
}

func (c *Client) GetScanProfile(ctx context.Context, id string) (*ScanProfile, error) {
	var out ScanProfile
	err := c.Do(ctx, http.MethodGet, "/api/profiles/"+url.PathEscape(id), nil, &out)
	return &out, err
}

func (c *Client) UpdateScanProfile(ctx context.Context, id string, in ScanProfile) (*ScanProfile, error) {
	var out ScanProfile
	err := c.Do(ctx, http.MethodPatch, "/api/profiles/"+url.PathEscape(id), in, &out)
	return &out, err
}

func (c *Client) DeleteScanProfile(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/profiles/"+url.PathEscape(id), nil, nil)
}

// ── Scans ────────────────────────────────────────────────────────────────────

type ScanSummary struct {
	RiskScore           float64        `json:"risk_score"`
	VulnerabilityCounts map[string]any `json:"vulnerability_counts"`
}

type Scan struct {
	ID          string      `json:"id"`
	TargetURL   string      `json:"target_url"`
	ScanType    string      `json:"scan_type"`
	Status      string      `json:"status"`
	StartedAt   *string     `json:"started_at"`
	CompletedAt *string     `json:"completed_at"`
	Summary     ScanSummary `json:"summary"`
}

// LatestCompletedScan is the newest completed scan of targetURL owned by the
// caller, or nil when there is none.
func (c *Client) LatestCompletedScan(ctx context.Context, targetURL string) (*Scan, error) {
	q := url.Values{}
	q.Set("target_url", targetURL)
	q.Set("status", "completed")
	q.Set("per_page", "1")
	q.Set("mine", "1")
	var out struct {
		Items []Scan `json:"items"`
	}
	if err := c.Do(ctx, http.MethodGet, "/api/scans?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	if len(out.Items) == 0 {
		return nil, nil
	}
	return &out.Items[0], nil
}
