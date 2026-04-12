package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func runDB(args []string) error {
	if len(args) == 0 {
		return errors.New("missing db subcommand (expected: migrate)")
	}
	switch args[0] {
	case "migrate":
		return runDBMigrate(args[1:])
	default:
		return fmt.Errorf("unknown db subcommand %q", args[0])
	}
}

func runDBMigrate(args []string) error {
	if len(args) == 0 {
		return errors.New("missing migrate subcommand (expected: status|apply)")
	}
	switch args[0] {
	case "status":
		return runDBMigrateStatus(args[1:])
	case "apply":
		return runDBMigrateApply(args[1:])
	default:
		return fmt.Errorf("unknown migrate subcommand %q", args[0])
	}
}

func runDBMigrateStatus(args []string) error {
	fs := flag.NewFlagSet("db migrate status", flag.ContinueOnError)
	cfgPath := fs.String("config", "apicerberus.yaml", "path to gateway config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	s, err := store.Open(cfg)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	applied, pending, err := s.MigrationStatus()
	if err != nil {
		return fmt.Errorf("get migration status: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tNAME\tSTATUS")
	for _, m := range applied {
		fmt.Fprintf(w, "%d\t%s\tapplied\n", m.Version, m.Name)
	}
	for _, m := range pending {
		fmt.Fprintf(w, "%d\t%spending\n", m.Version, m.Name)
	}
	w.Flush()

	fmt.Printf("\n%d applied, %d pending\n", len(applied), len(pending))
	return nil
}

func runDBMigrateApply(args []string) error {
	fs := flag.NewFlagSet("db migrate apply", flag.ContinueOnError)
	cfgPath := fs.String("config", "apicerberus.yaml", "path to gateway config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	s, err := store.Open(cfg)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	_, pending, err := s.MigrationStatus()
	if err != nil {
		return fmt.Errorf("get migration status: %w", err)
	}

	if len(pending) == 0 {
		fmt.Println("All migrations are already applied.")
		return nil
	}

	fmt.Printf("Applying %d pending migration(s)...\n", len(pending))
	for _, m := range pending {
		fmt.Printf("  v%d: %s\n", m.Version, m.Name)
	}

	// Re-running store.Open already applied migrations during open.
	// Just report the final status.
	applied, _, err := s.MigrationStatus()
	if err != nil {
		return fmt.Errorf("get migration status: %w", err)
	}

	fmt.Printf("\nDone. %d applied, %d pending\n", len(applied), len(pending))
	if len(pending) > 0 {
		return errors.New("some migrations are still pending")
	}
	return nil
}
