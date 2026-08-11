package dbcore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Migration represents a single SQL schema migration statement.
type Migration struct {
	ID      string
	Version int
	Name    string
	SQL     string
	UpSQL   string
	DownSQL string
}

// AutoMigrate runs schema migrations transactional with tracking in gpp_migrations table.
func AutoMigrate(ctx context.Context, client *Client, migrations ...Migration) error {
	if client == nil {
		return fmt.Errorf("dbcore/migrate: database client is nil")
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS gpp_migrations (
		id VARCHAR(255) PRIMARY KEY,
		version INT NOT NULL,
		name VARCHAR(255) NOT NULL,
		checksum VARCHAR(64) NOT NULL,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := client.Exec(ctx, createTableSQL); err != nil {
		return fmt.Errorf("dbcore/migrate: failed to initialize gpp_migrations table: %w", err)
	}

	applied := make(map[string]bool)
	err := client.Query(ctx, "SELECT id FROM gpp_migrations", func(rows *sql.Rows) error {
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				applied[id] = true
			}
		}
		return rows.Err()
	})
	if err != nil {
		return fmt.Errorf("dbcore/migrate: failed to query applied migrations: %w", err)
	}

	for i, m := range migrations {
		if m.ID == "" {
			if m.Version > 0 {
				m.ID = fmt.Sprintf("%04d_%s", m.Version, sanitizeName(m.Name))
			} else {
				m.ID = fmt.Sprintf("%04d_%s", i+1, sanitizeName(m.Name))
			}
		}

		if applied[m.ID] {
			continue
		}

		sqlContent := m.SQL
		if sqlContent == "" {
			sqlContent = m.UpSQL
		}
		if strings.TrimSpace(sqlContent) == "" {
			continue
		}

		hash := sha256.Sum256([]byte(sqlContent))
		checksum := hex.EncodeToString(hash[:])

		err := client.InTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, sqlContent); err != nil {
				return fmt.Errorf("migration statement failed: %w", err)
			}
			insertSQL := "INSERT INTO gpp_migrations (id, version, name, checksum, applied_at) VALUES ($1, $2, $3, $4, $5)"
			_, err := tx.ExecContext(ctx, insertSQL, m.ID, m.Version, m.Name, checksum, time.Now())
			return err
		})

		if err != nil {
			return fmt.Errorf("dbcore/migrate: failed migration '%s': %w", m.ID, err)
		}

		slog.Info("dbcore/migrate: Applied schema migration", slog.String("id", m.ID), slog.String("checksum", checksum[:8]))
	}

	return nil
}

// Migrate is a helper alias executing pending migrations.
func Migrate(ctx context.Context, client *Client, migrations []Migration) error {
	return AutoMigrate(ctx, client, migrations...)
}

// MigrateEmbed reads and executes .sql files from an embedded fs.FS filesystem.
func MigrateEmbed(ctx context.Context, client *Client, embedFS fs.FS, dir string) error {
	entries, err := fs.ReadDir(embedFS, dir)
	if err != nil {
		return fmt.Errorf("dbcore/migrate: failed reading embedded directory '%s': %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	var migrations []Migration
	for i, filename := range files {
		path := filepath.Join(dir, filename)
		content, err := fs.ReadFile(embedFS, path)
		if err != nil {
			return fmt.Errorf("dbcore/migrate: error reading embedded file '%s': %w", path, err)
		}

		name := strings.TrimSuffix(filename, ".sql")
		version := i + 1
		parts := strings.SplitN(name, "_", 2)
		if len(parts) == 2 {
			if v, err := strconv.Atoi(parts[0]); err == nil {
				version = v
				name = parts[1]
			}
		}

		migrations = append(migrations, Migration{
			ID:      filename,
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	return AutoMigrate(ctx, client, migrations...)
}

func sanitizeName(s string) string {
	if s == "" {
		return "migration"
	}
	return strings.ReplaceAll(strings.ToLower(s), " ", "_")
}
