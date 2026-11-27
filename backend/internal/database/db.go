package database

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect abre a conexão e aguarda o MySQL ficar disponível (útil no boot via compose).
func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	var lastErr error
	for i := 0; i < 30; i++ {
		if lastErr = db.Ping(); lastErr == nil {
			return db, nil
		}
		log.Printf("aguardando MySQL (%d/30): %v", i+1, lastErr)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("MySQL indisponível: %w", lastErr)
}

// Migrate aplica, em ordem, os arquivos migrations/*.sql ainda não registrados.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(100) PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		raw, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		for _, stmt := range strings.Split(string(raw), ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			return err
		}
		log.Printf("migration aplicada: %s", name)
	}
	return nil
}
