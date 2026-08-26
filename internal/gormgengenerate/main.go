package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/imbrooklyn/weave-integration-testbed/internal/sqlgorm"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"gorm.io/gen"
)

const generationTimeout = 2 * time.Minute

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "generate GORM Gen fixture: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("locate repository root: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), generationTimeout)
	defer cancel()

	database, cleanup, err := sqlgorm.Open(ctx, testenv.MySQL)
	if err != nil {
		return err
	}
	defer cleanup()

	generator := gen.NewGenerator(gen.Config{
		OutPath:       filepath.Join(root, "internal", "gormgenquery"),
		ModelPkgPath:  filepath.Join(root, "internal", "gormgenmodel"),
		FieldNullable: true,
		FieldSignable: true,
		Mode: gen.WithoutContext |
			gen.WithQueryInterface,
	})
	generator.UseDB(database.WithContext(ctx))
	model := generator.GenerateModelAs("semantic_records", "SemanticRecord")
	if model == nil {
		return fmt.Errorf("semantic_records did not produce a model")
	}
	generator.ApplyBasic(model)
	generator.Execute()
	return nil
}
