package smithsonian

import (
	"context"
	"fmt"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the smithsonian driver.
type Domain struct{}

// Info describes the scheme, hostnames, and binary identity.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "smithsonian",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "smithsonian",
			Short:  "A command line for the Smithsonian Open Access API.",
			Long: `A command line for the Smithsonian Open Access API.

smithsonian searches 14.4 million museum objects from natural history, art,
space, and American history over HTTPS. No registration required — the public
DEMO_KEY works out of the box.`,
			Site: Host,
			Repo: "https://github.com/tamnd/smithsonian-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search Smithsonian museum objects",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}}}, searchObjects)

	kit.Handle(app, kit.OpMeta{Name: "stats", Group: "read", List: true,
		Summary: "Show object counts per Smithsonian unit"}, getStats)
}

// newClient builds the Client from kit config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- input structs ---

type searchInput struct {
	Query  string  `kit:"arg" help:"search query"`
	Rows   int     `kit:"flag,inherit" help:"results per page (default 25)"`
	Start  int     `kit:"flag" help:"pagination offset (0-based)"`
	Client *Client `kit:"inject"`
}

type statsInput struct {
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchObjects(ctx context.Context, in searchInput, emit func(*Object) error) error {
	rows := in.Rows
	if rows <= 0 {
		rows = 25
	}
	items, err := in.Client.Search(ctx, in.Query, rows, in.Start)
	if err != nil {
		return err
	}
	for i := range items {
		if err := emit(&items[i]); err != nil {
			return err
		}
	}
	return nil
}

func getStats(ctx context.Context, in statsInput, emit func(*UnitStat) error) error {
	items, err := in.Client.Stats(ctx)
	if err != nil {
		return err
	}
	for i := range items {
		if err := emit(&items[i]); err != nil {
			return err
		}
	}
	return nil
}

// Classify turns any string into (type, id).
func (Domain) Classify(input string) (string, string, error) {
	return "object", input, nil
}

// Locate returns the live https URL for a (type, id).
func (Domain) Locate(t, id string) (string, error) {
	switch t {
	case "object":
		return fmt.Sprintf("https://collections.si.edu/search/detail/%s", id), nil
	default:
		return "", errs.Usage("smithsonian has no resource type %q", t)
	}
}
