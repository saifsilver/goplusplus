package gpp_test

import (
	"context"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/dbcore/seed"
)

func TestHandleCLIMigrateAndSeed(t *testing.T) {
	app := gpp.New()

	ctx := context.Background()
	client, err := dbcore.NewClient(ctx, dbcore.Config{RWDSN: ":memory:"})
	if err != nil {
		t.Fatalf("failed creating memory db client: %v", err)
	}

	opts := gpp.CLIOptions{
		Client: client,
		Migrations: []dbcore.Migration{
			{Version: 1, Name: "init_users", SQL: "CREATE TABLE users (id INT, name TEXT);"},
		},
		SeedPlans: []seed.Plan{
			{Table: "users", Count: 3, Factory: func(f *seed.Faker) map[string]any {
				return map[string]any{"id": f.IntRange(1, 100), "name": f.Name()}
			}},
		},
		Args: []string{"app", "migrate"},
	}

	if !app.HandleCLI(opts) {
		t.Errorf("expected HandleCLI to handle 'migrate' command")
	}

	opts.Args = []string{"app", "seed"}
	if !app.HandleCLI(opts) {
		t.Errorf("expected HandleCLI to handle 'seed' command")
	}

	opts.Args = []string{"app", "migrate:fresh"}
	if !app.HandleCLI(opts) {
		t.Errorf("expected HandleCLI to handle 'migrate:fresh' command")
	}

	opts.Args = []string{"app"}
	if app.HandleCLI(opts) {
		t.Errorf("expected HandleCLI to return false when no CLI command is passed")
	}
}
