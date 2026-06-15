package smithsonian_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/smithsonian-cli/smithsonian"
)

func newTestClient(ts *httptest.Server) *smithsonian.Client {
	cfg := smithsonian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return smithsonian.NewClient(cfg)
}

// TestSearch checks that search results are parsed correctly.
func TestSearch(t *testing.T) {
	fixture := map[string]any{
		"response": map[string]any{
			"rowCount": 1,
			"rows": []any{
				map[string]any{
					"id":       "ld1-123",
					"title":    "Eagle / Janine Rogers",
					"unitCode": "SIL",
					"type":     "edanmdm",
					"url":      "edanmdm:siris_sil_1052006",
					"content": map[string]any{
						"descriptiveNonRepeating": map[string]any{
							"title":       map[string]any{"content": "Eagle / Janine Rogers"},
							"unit_code":   "SIL",
							"data_source": "Smithsonian Libraries",
							"record_link": "https://ids.si.edu/ids/deliveryService?id=SIL-img-001",
							"metadata_usage": map[string]any{
								"access": "CC0",
							},
						},
					},
				},
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(fixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	objs, err := c.Search(context.Background(), "eagle", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 {
		t.Fatalf("got %d objects, want 1", len(objs))
	}
	obj := objs[0]
	if obj.ID != "ld1-123" {
		t.Errorf("ID = %q, want ld1-123", obj.ID)
	}
	if obj.Title != "Eagle / Janine Rogers" {
		t.Errorf("Title = %q", obj.Title)
	}
	if obj.UnitCode != "SIL" {
		t.Errorf("UnitCode = %q, want SIL", obj.UnitCode)
	}
	if obj.DataSource != "Smithsonian Libraries" {
		t.Errorf("DataSource = %q", obj.DataSource)
	}
	if obj.Access != "CC0" {
		t.Errorf("Access = %q, want CC0", obj.Access)
	}
	if obj.RecordLink == "" {
		t.Error("RecordLink should not be empty")
	}
}

// TestSearchAPIKey checks that the api_key param is sent with every search request.
func TestSearchAPIKey(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		resp := map[string]any{
			"response": map[string]any{"rowCount": 0, "rows": []any{}},
		}
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.Search(context.Background(), "moon", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "api_key=DEMO_KEY") {
		t.Errorf("query %q should contain api_key=DEMO_KEY", gotQuery)
	}
}

// TestStats checks that the stats response is parsed into unit list.
func TestStats(t *testing.T) {
	fixture := map[string]any{
		"response": map[string]any{
			"total_objects": 14468638,
			"units": []any{
				map[string]any{
					"unit":          "NASM",
					"data_source":   "National Air and Space Museum",
					"total_objects": 9058,
					"metrics": map[string]any{
						"CC0_records": 8100,
					},
				},
				map[string]any{
					"unit":          "SIL",
					"data_source":   "Smithsonian Libraries",
					"total_objects": 302894,
					"metrics": map[string]any{
						"CC0_records": 290000,
					},
				},
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(fixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	units, err := c.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2", len(units))
	}
	if units[0].Unit != "NASM" {
		t.Errorf("units[0].Unit = %q, want NASM", units[0].Unit)
	}
	if units[0].Total != 9058 {
		t.Errorf("units[0].Total = %d, want 9058", units[0].Total)
	}
	if units[0].CC0 != 8100 {
		t.Errorf("units[0].CC0 = %d, want 8100", units[0].CC0)
	}
}

// TestRetryOn503 checks that the client retries on 503 and succeeds eventually.
func TestRetryOn503(t *testing.T) {
	var hits int
	fixture := map[string]any{
		"response": map[string]any{
			"total_objects": 100,
			"units": []any{
				map[string]any{
					"unit":          "NASM",
					"data_source":   "National Air and Space Museum",
					"total_objects": 9058,
					"metrics": map[string]any{
						"CC0_records": 8100,
					},
				},
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		b, _ := json.Marshal(fixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	cfg := smithsonian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := smithsonian.NewClient(cfg)

	start := time.Now()
	_, err := c.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}
