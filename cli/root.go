// Package cli assembles the smithsonian command tree from the smithsonian
// domain on top of the any-cli/kit framework.
package cli

import (
	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/smithsonian-cli/smithsonian"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// NewApp assembles the kit application from the smithsonian domain. The
// domain's Register installs the client factory and every operation, so the
// binary and a host (ant, which blank-imports the package) share one source of
// truth. kit.Run turns the App into the CLI, plus the serve and mcp surfaces and
// the typed-error-to-exit-code mapping.
//
// To add a command, declare it in smithsonian/domain.go with kit.Handle and it
// appears here automatically. Reach for app.AddCommand only for a verb that does
// not fit the emit-records shape, the way version does below.
func NewApp() *kit.App {
	id := smithsonian.Domain{}.Info().Identity
	id.Version = Version

	app := kit.New(id)
	(smithsonian.Domain{}).Register(app)
	app.AddCommand(newVersionCmd())
	return app
}
