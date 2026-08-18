package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"anonpass/internal/migrate"
)

func main() {
	dsn := flag.String("dsn", "", "PostgreSQL DSN")
	dir := flag.String("dir", "migrations", "directory with .sql migration files")
	timeout := flag.Duration("timeout", 30*time.Second, "migration timeout")
	flag.Parse()

	if *dsn == "" {
		log.Fatal("missing dsn")
	}

	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	if err := migrate.ApplyDir(ctx, db, *dir); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}
	log.Printf("applied migrations from %s", *dir)
}
