package routes

import (
	"time"

	"github.com/gin-gonic/gin"

	"shortlink/handlers"
	"shortlink/middleware"
)

// SetupRoutes mendaftarkan semua endpoint API
func SetupRoutes(r *gin.Engine) {
	// Public routes
	r.POST("/auth/register", handlers.Register)
	r.POST("/auth/login", handlers.Login)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Redirect endpoint - public, dengan rate limiting (60 request per menit per IP)
	r.GET("/:code", middleware.RateLimit(60, time.Minute), handlers.RedirectLink)

	// Stats & QR code - public juga, biar bisa dicek tanpa login
	r.GET("/:code/stats", handlers.GetLinkStats)
	r.GET("/:code/qr", handlers.GetQRCode)

	// Protected routes
	auth := r.Group("/")
	auth.Use(middleware.AuthRequired())
	{
		// Rate limit pembuatan link: max 10 per menit per IP, mencegah abuse
		auth.POST("/shorten", middleware.RateLimit(10, time.Minute), handlers.CreateLink)
		auth.GET("/my-links", handlers.GetMyLinks)
		auth.DELETE("/:code", handlers.DeleteLink)
	}
}
