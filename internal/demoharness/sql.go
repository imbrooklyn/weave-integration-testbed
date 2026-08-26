package demoharness

import (
	"fmt"

	"github.com/imbrooklyn/weave"
	weavegorm "github.com/imbrooklyn/weave-adapters/gorm"
	"github.com/imbrooklyn/weave-adapters/gormgen"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
)

var equalityOnlyOperators = []weave.Operator{
	weave.OperatorEQ,
	weave.OperatorNEQ,
	weave.OperatorIn,
	weave.OperatorNotIn,
	weave.OperatorIsNull,
	weave.OperatorNotNull,
}

func gormProfile(backend testenv.Backend) (weavegorm.Profile, error) {
	switch backend {
	case testenv.MySQL:
		return weavegorm.MySQL, nil
	case testenv.PostgreSQL:
		return weavegorm.PostgreSQL, nil
	default:
		return 0, fmt.Errorf("unsupported SQL backend %q", backend)
	}
}

func gormgenProfile(backend testenv.Backend) (gormgen.Profile, error) {
	switch backend {
	case testenv.MySQL:
		return gormgen.MySQL, nil
	case testenv.PostgreSQL:
		return gormgen.PostgreSQL, nil
	default:
		return 0, fmt.Errorf("unsupported SQL backend %q", backend)
	}
}
