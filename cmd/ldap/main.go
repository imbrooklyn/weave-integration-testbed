// Command ldap demonstrates the Weave LDAP Adapter against a real OpenLDAP
// server from the directory Compose profile.
package main

import (
	"os"

	"github.com/imbrooklyn/weave-integration-testbed/internal/demoapp"
	"github.com/imbrooklyn/weave-integration-testbed/internal/demoharness"
)

func main() {
	os.Exit(demoapp.RunDocument(
		"ldap",
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		demoharness.RunLDAP,
	))
}
