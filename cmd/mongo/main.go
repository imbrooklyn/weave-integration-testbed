// Command mongo demonstrates the Weave MongoDB Adapter against a real server.
package main

import (
	"os"

	"github.com/imbrooklyn/weave-integration-testbed/internal/demoapp"
	"github.com/imbrooklyn/weave-integration-testbed/internal/demoharness"
)

func main() {
	os.Exit(demoapp.RunDocument(
		"mongo",
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		demoharness.RunMongo,
	))
}
