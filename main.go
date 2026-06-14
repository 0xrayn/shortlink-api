package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"shortlink/config"
	"shortlink/routes"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Tidak menemukan file .env, menggunakan environment variable sistem")
	}

	config.ConnectDB()
	config.ConnectRedis()
	config.RunMigrations()

	r := gin.Default()
	routes.SetupRoutes(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Gagal menjalankan server: %v", err)
	}
}
