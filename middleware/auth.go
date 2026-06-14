package middleware

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"shortlink/config"
)

// AuthRequired memvalidasi JWT token dari header Authorization
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header diperlukan"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_SECRET")), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token tidak valid"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token tidak valid"})
			return
		}

		c.Set("user_id", int(claims["user_id"].(float64)))
		c.Next()
	}
}

// RateLimit membatasi jumlah request per IP dalam jangka waktu tertentu menggunakan Redis.
// Implementasi fixed-window counter: setiap key punya TTL, kalau counter melebihi limit, request ditolak.
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := "ratelimit:" + c.FullPath() + ":" + ip

		count, err := config.RDB.Incr(config.Ctx, key).Result()
		if err != nil {
			// Kalau Redis error, jangan blokir request - fail open
			c.Next()
			return
		}

		if count == 1 {
			config.RDB.Expire(config.Ctx, key, window)
		}

		if count > int64(limit) {
			ttl, _ := config.RDB.TTL(config.Ctx, key).Result()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "Terlalu banyak request, coba lagi nanti",
				"retry_after": ttl.Seconds(),
			})
			return
		}

		c.Next()
	}
}
