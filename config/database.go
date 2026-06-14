package config

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/redis/go-redis/v9"

	_ "github.com/lib/pq"
)

var DB *sql.DB
var RDB *redis.Client
var Ctx = context.Background()

const schemaSQL = `
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS links (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) UNIQUE NOT NULL,
    original_url TEXT NOT NULL,
    user_id INT REFERENCES users(id),
    click_count BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS link_visits (
    id SERIAL PRIMARY KEY,
    link_id INT NOT NULL REFERENCES links(id),
    user_agent TEXT,
    ip_address VARCHAR(64),
    visited_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_links_code ON links(code);
CREATE INDEX IF NOT EXISTS idx_link_visits_link_id ON link_visits(link_id);
`

// ConnectDB membuka koneksi ke PostgreSQL
func ConnectDB() {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Gagal membuka koneksi database: %v", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatalf("Gagal konek ke database: %v", err)
	}

	DB = db
	log.Println("PostgreSQL connected successfully")
}

// ConnectRedis membuka koneksi ke Redis
func ConnectRedis() {
	addr := fmt.Sprintf("%s:%s", os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT"))

	RDB = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	if err := RDB.Ping(Ctx).Err(); err != nil {
		log.Fatalf("Gagal konek ke Redis: %v", err)
	}

	log.Println("Redis connected successfully")
}

// RunMigrations membuat tabel kalau belum ada
func RunMigrations() {
	if _, err := DB.Exec(schemaSQL); err != nil {
		log.Fatalf("Gagal menjalankan migration: %v", err)
	}
	log.Println("Migration berhasil, semua tabel siap")
}
