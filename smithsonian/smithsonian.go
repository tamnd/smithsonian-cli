// Package smithsonian is the library behind the smithsonian command line:
// the HTTP client, request shaping, and typed data models for the Smithsonian
// Open Access API (https://api.si.edu/openaccess/api/v1.0).
//
// The demo API key (DEMO_KEY) works without registration. The Client paces
// requests, sets a real User-Agent, and retries transient failures (429 and
// 5xx).
package smithsonian

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Host is the API hostname.
const Host = "api.si.edu"

// Config holds all tunable parameters for the Client.
type Config struct {
	BaseURL   string
	APIKey    string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.si.edu/openaccess/api/v1.0",
		APIKey:    "DEMO_KEY",
		UserAgent: "smithsonian-cli/0.1 (+https://github.com/tamnd/smithsonian-cli)",
		Rate:      500 * time.Millisecond,
		Timeout:   15 * time.Second,
		Retries:   3,
	}
}

// Object holds the public data about a single Smithsonian museum object.
type Object struct {
	ID         string `kit:"id" json:"id"`
	Title      string `json:"title"`
	UnitCode   string `json:"unit_code"`
	DataSource string `json:"data_source"`
	Type       string `json:"type"`
	RecordLink string `json:"record_link,omitempty"`
	Access     string `json:"access,omitempty"`
}

// UnitStat holds object counts for a single Smithsonian unit.
type UnitStat struct {
	Unit       string `kit:"id" json:"unit"`
	DataSource string `json:"data_source"`
	Total      int    `json:"total_objects"`
	CC0        int    `json:"cc0_records"`
}

// --- wire types ---

type wireRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	UnitCode string `json:"unitCode"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	Content  struct {
		DescriptiveNonRepeating struct {
			Title      *struct{ Content string `json:"content"` } `json:"title"`
			UnitCode   string                                     `json:"unit_code"`
			DataSource string                                     `json:"data_source"`
			RecordLink string                                     `json:"record_link"`
			MetadataUsage *struct {
				Access string `json:"access"`
			} `json:"metadata_usage"`
		} `json:"descriptiveNonRepeating"`
	} `json:"content"`
}

type wireSearchResp struct {
	Response struct {
		RowCount int       `json:"rowCount"`
		Rows     []wireRow `json:"rows"`
	} `json:"response"`
}

type wireUnit struct {
	Unit       string `json:"unit"`
	DataSource string `json:"data_source"`
	Total      int    `json:"total_objects"`
	Metrics    struct {
		CC0Records int `json:"CC0_records"`
	} `json:"metrics"`
}

type wireStatsResp struct {
	Response struct {
		Units        []wireUnit `json:"units"`
		TotalObjects int        `json:"total_objects"`
	} `json:"response"`
}

// Client talks to the Smithsonian Open Access API.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client with the given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Search searches museum objects matching query.
func (c *Client) Search(ctx context.Context, query string, rows, start int) ([]Object, error) {
	if rows <= 0 {
		rows = 25
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("api_key", c.cfg.APIKey)
	params.Set("rows", fmt.Sprintf("%d", rows))
	params.Set("start", fmt.Sprintf("%d", start))

	rawURL := c.cfg.BaseURL + "/search?" + params.Encode()
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var resp wireSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse search: %w", err)
	}
	return flattenRows(resp.Response.Rows), nil
}

// Stats returns object counts per Smithsonian unit.
func (c *Client) Stats(ctx context.Context) ([]UnitStat, error) {
	params := url.Values{}
	params.Set("api_key", c.cfg.APIKey)

	rawURL := c.cfg.BaseURL + "/stats?" + params.Encode()
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var resp wireStatsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse stats: %w", err)
	}
	return flattenUnits(resp.Response.Units), nil
}

// get fetches a URL and returns the body, pacing and retrying as configured.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- flatten helpers ---

func flattenRow(w wireRow) Object {
	dnr := w.Content.DescriptiveNonRepeating
	title := w.Title
	if dnr.Title != nil && dnr.Title.Content != "" {
		title = dnr.Title.Content
	}
	access := ""
	if dnr.MetadataUsage != nil {
		access = dnr.MetadataUsage.Access
	}
	unitCode := dnr.UnitCode
	if unitCode == "" {
		unitCode = w.UnitCode
	}
	return Object{
		ID:         w.ID,
		Title:      title,
		UnitCode:   unitCode,
		DataSource: dnr.DataSource,
		Type:       w.Type,
		RecordLink: dnr.RecordLink,
		Access:     access,
	}
}

func flattenRows(ws []wireRow) []Object {
	out := make([]Object, len(ws))
	for i, w := range ws {
		out[i] = flattenRow(w)
	}
	return out
}

func flattenUnits(ws []wireUnit) []UnitStat {
	out := make([]UnitStat, len(ws))
	for i, w := range ws {
		out[i] = UnitStat{
			Unit:       w.Unit,
			DataSource: w.DataSource,
			Total:      w.Total,
			CC0:        w.Metrics.CC0Records,
		}
	}
	return out
}
