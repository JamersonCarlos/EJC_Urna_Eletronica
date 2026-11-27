// Seed: aplica as migrations e cria o usuário mesário/administrador inicial.
// Opcional: SEED_DEMO=true cria uma eleição de demonstração.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/agreser-innovations/urna-backend/internal/auth"
	"github.com/agreser-innovations/urna-backend/internal/config"
	"github.com/agreser-innovations/urna-backend/internal/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Connect(cfg.DSN())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		log.Fatal(err)
	}

	name := envOr("ADMIN_NAME", "Mesário")
	email := strings.ToLower(envOr("ADMIN_EMAIL", "admin@agreser.tech"))
	pass := os.Getenv("ADMIN_PASSWORD")

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE email=?`, email).Scan(&count); err != nil {
		log.Fatal(err)
	}
	if count == 0 {
		if pass == "" {
			pass, _ = auth.RandomToken(8)
			log.Printf("ADMIN_PASSWORD não definido; senha gerada: %s (troque após o primeiro login)", pass)
		}
		hash, err := auth.HashPassword(pass)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO users (name, email, password_hash) VALUES (?,?,?)`, name, email, hash); err != nil {
			log.Fatal(err)
		}
		log.Printf("usuário criado: %s", email)
	} else {
		log.Printf("usuário %s já existe — nada a fazer", email)
	}

	if os.Getenv("SEED_DEMO") == "true" {
		if err := seedDemo(db); err != nil {
			log.Fatal(err)
		}
	}
	log.Println("seed concluído")
}

func seedDemo(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM elections WHERE title='Eleição de Demonstração'`).Scan(&n); err != nil || n > 0 {
		return err
	}
	res, err := db.Exec(`INSERT INTO elections (title, description, office, digits) VALUES ('Eleição de Demonstração', 'Criada pelo seed', 'Representante', 2)`)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	for _, c := range [][2]string{{"01", "Ana Souza"}, {"02", "Bruno Lima"}, {"03", "Carla Mendes"}} {
		if _, err := db.Exec(`INSERT INTO candidates (election_id, number, name) VALUES (?,?,?)`, id, c[0], c[1]); err != nil {
			return err
		}
	}
	for i := 1; i <= 10; i++ {
		if _, err := db.Exec(`INSERT INTO voters (election_id, name, document) VALUES (?,?,?)`, id, fmt.Sprintf("Participante %02d", i), fmt.Sprintf("E%04d", i)); err != nil {
			return err
		}
	}
	log.Printf("eleição de demonstração criada (id %d)", id)
	return nil
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
