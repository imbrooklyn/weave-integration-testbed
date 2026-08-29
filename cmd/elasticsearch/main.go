// Command elasticsearch demonstrates the Weave Elasticsearch Adapter against
// the exact real server locked by the search profile.
package main

import (
	"os"

	"github.com/imbrooklyn/weave-integration-testbed/internal/demoapp"
	"github.com/imbrooklyn/weave-integration-testbed/internal/demoharness"
)

func main() {
	os.Exit(demoapp.RunDocument(
		"elasticsearch",
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		demoharness.RunElasticsearch,
	))
}
