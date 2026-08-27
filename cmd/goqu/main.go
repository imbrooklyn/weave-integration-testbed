package main

import (
	"os"

	"github.com/imbrooklyn/weave-integration-testbed/internal/demoapp"
	"github.com/imbrooklyn/weave-integration-testbed/internal/demoharness"
)

func main() {
	os.Exit(demoapp.RunSQL(
		"goqu",
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		demoharness.RunGoqu,
	))
}
