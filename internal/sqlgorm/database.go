// Package sqlgorm opens the local SQL services through public GORM APIs.
package sqlgorm

import (
	"context"
	"fmt"
	"time"

	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	gormmysql "gorm.io/driver/mysql"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open waits for backend and returns a GORM handle plus a cleanup function.
// The cleanup function closes the underlying database/sql connection pool.
func Open(
	ctx context.Context,
	backend testenv.Backend,
) (*gorm.DB, func() error, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("open %s GORM database: nil context", backend)
	}
	config, err := testenv.LoadSQLConfig(backend)
	if err != nil {
		return nil, nil, err
	}
	sqlDatabase, err := testenv.OpenSQL(config)
	if err != nil {
		return nil, nil, err
	}
	cleanup := sqlDatabase.Close
	if err := testenv.WaitForSQL(ctx, backend, sqlDatabase, 250*time.Millisecond); err != nil {
		_ = cleanup()
		return nil, nil, err
	}

	var dialector gorm.Dialector
	switch backend {
	case testenv.MySQL:
		dialector = gormmysql.New(gormmysql.Config{
			Conn:                      sqlDatabase,
			SkipInitializeWithVersion: true,
		})
	case testenv.PostgreSQL:
		dialector = gormpostgres.New(gormpostgres.Config{
			Conn:                 sqlDatabase,
			PreferSimpleProtocol: true,
		})
	default:
		_ = cleanup()
		return nil, nil, fmt.Errorf("unsupported SQL backend %q", backend)
	}

	database, err := gorm.Open(dialector, &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = cleanup()
		return nil, nil, fmt.Errorf("open %s GORM session: %w", backend, err)
	}
	return database, cleanup, nil
}
