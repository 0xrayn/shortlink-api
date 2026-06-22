package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode"

	"shortlink/config"
	"shortlink/models"
)

const cacheTTL = 1 * time.Hour

// generateCode membuat random short code 6 karakter (base64 url-safe)
func generateCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := base64.RawURLEncoding.EncodeToString(b)
	return code[:6], nil
}

// CreateLink membuat short link baru. Kalau "code" diisi, dipakai sebagai custom alias.
func CreateLink(c *gin.Context) {
	var req models.CreateLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")

	code := strings.TrimSpace(req.Code)
	if code == "" {
		// Generate random code, retry kalau collision (jarang terjadi)
		for i := 0; i < 5; i++ {
			generated, err := generateCode()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal generate kode"})
				return
			}
			var exists bool
			config.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM links WHERE code = $1)`, generated).Scan(&exists)
			if !exists {
				code = generated
				break
			}
		}
	}

	var id int
	var createdAt time.Time
	err := config.DB.QueryRow(
		`INSERT INTO links (code, original_url, user_id) VALUES ($1, $2, $3) RETURNING id, created_at`,
		code, req.URL, userID,
	).Scan(&id, &createdAt)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code sudah dipakai, coba code lain"})
		return
	}

	// Simpan mapping ke Redis langsung supaya redirect pertama sudah cache hit
	cacheVal := strconv.Itoa(id) + "|" + req.URL
	config.RDB.Set(config.Ctx, "link:"+code, cacheVal, cacheTTL)

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":           id,
		"code":         code,
		"short_url":    baseURL + "/" + code,
		"original_url": req.URL,
		"created_at":   createdAt,
	})
}

// RedirectLink melakukan redirect dari short code ke URL asli.
// Menggunakan cache-aside pattern: cek Redis dulu, kalau miss baru query database lalu simpan ke cache.
func RedirectLink(c *gin.Context) {
	code := c.Param("code")

	// 1. Cek cache Redis dulu
	cachedVal, err := config.RDB.Get(config.Ctx, "link:"+code).Result()

	var linkID int
	var originalURL string

	if err != nil {
		// 2. Cache miss -> query database
		err = config.DB.QueryRow(
			`SELECT id, original_url FROM links WHERE code = $1`, code,
		).Scan(&linkID, &originalURL)

		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Link tidak ditemukan"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data link"})
			return
		}

		// 3. Simpan ke cache untuk request selanjutnya
		cacheVal := strconv.Itoa(linkID) + "|" + originalURL
		config.RDB.Set(config.Ctx, "link:"+code, cacheVal, cacheTTL)
	} else {
		// Cache hit - coba parse format `id|url`
		parts := strings.SplitN(cachedVal, "|", 2)
		if len(parts) == 2 {
			if id, parseErr := strconv.Atoi(parts[0]); parseErr == nil {
				linkID = id
				originalURL = parts[1]
			}
		}

		// Fallback jika format cache tidak cocok (misal: data cache lama yang hanya berisi URL)
		if linkID == 0 || originalURL == "" {
			err = config.DB.QueryRow(
				`SELECT id, original_url FROM links WHERE code = $1`, code,
			).Scan(&linkID, &originalURL)

			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Link tidak ditemukan"})
				return
			} else if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data link"})
				return
			}

			// Simpan ulang ke cache dengan format baru
			cacheVal := strconv.Itoa(linkID) + "|" + originalURL
			config.RDB.Set(config.Ctx, "link:"+code, cacheVal, cacheTTL)
		}
	}

	// Increment click count secara async-ish (fire and forget untuk performa redirect)
	go recordVisit(linkID, c.ClientIP(), c.GetHeader("User-Agent"))

	c.Redirect(http.StatusFound, originalURL)
}

// recordVisit mencatat history klik dan increment counter di database
func recordVisit(linkID int, ip, userAgent string) {
	config.DB.Exec(
		`INSERT INTO link_visits (link_id, ip_address, user_agent) VALUES ($1, $2, $3)`,
		linkID, ip, userAgent,
	)
	config.DB.Exec(`UPDATE links SET click_count = click_count + 1 WHERE id = $1`, linkID)
}

// GetLinkStats mengembalikan statistik klik untuk sebuah short code
func GetLinkStats(c *gin.Context) {
	code := c.Param("code")

	var link models.Link
	err := config.DB.QueryRow(
		`SELECT id, code, original_url, user_id, click_count, created_at FROM links WHERE code = $1`,
		code,
	).Scan(&link.ID, &link.Code, &link.OriginalURL, &link.UserID, &link.ClickCount, &link.CreatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Link tidak ditemukan"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data link"})
		return
	}

	rows, err := config.DB.Query(
		`SELECT visited_at, ip_address, user_agent FROM link_visits 
		 WHERE link_id = $1 ORDER BY visited_at DESC LIMIT 20`,
		link.ID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil history klik"})
		return
	}
	defer rows.Close()

	visits := []gin.H{}
	for rows.Next() {
		var visitedAt time.Time
		var ip, ua string
		if err := rows.Scan(&visitedAt, &ip, &ua); err == nil {
			visits = append(visits, gin.H{"visited_at": visitedAt, "ip_address": ip, "user_agent": ua})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":          link.Code,
		"original_url":  link.OriginalURL,
		"click_count":   link.ClickCount,
		"created_at":    link.CreatedAt,
		"recent_visits": visits,
	})
}

// GetMyLinks mengambil daftar link milik user yang sedang login, dengan pagination
func GetMyLinks(c *gin.Context) {
	userID, _ := c.Get("user_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit

	var total int
	config.DB.QueryRow(`SELECT COUNT(*) FROM links WHERE user_id = $1`, userID).Scan(&total)

	rows, err := config.DB.Query(
		`SELECT id, code, original_url, user_id, click_count, created_at 
		 FROM links WHERE user_id = $1 ORDER BY id DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data link"})
		return
	}
	defer rows.Close()

	links := []models.Link{}
	for rows.Next() {
		var l models.Link
		if err := rows.Scan(&l.ID, &l.Code, &l.OriginalURL, &l.UserID, &l.ClickCount, &l.CreatedAt); err == nil {
			links = append(links, l)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": links,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetQRCode menghasilkan QR code (PNG) yang mengarah ke short URL.
// Bisa langsung dibuka di browser karena response Content-Type-nya image/png.
func GetQRCode(c *gin.Context) {
	code := c.Param("code")

	var exists bool
	if err := config.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM links WHERE code = $1)`, code).Scan(&exists); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memeriksa link"})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Link tidak ditemukan"})
		return
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	shortURL := baseURL + "/" + code

	png, err := qrcode.Encode(shortURL, qrcode.Medium, 256)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal generate QR code"})
		return
	}

	c.Data(http.StatusOK, "image/png", png)
}

// DeleteLink menghapus short link milik user yang sedang login.
// Hanya pemilik link yang bisa menghapus - link milik user lain akan return 404
// (bukan 403) supaya tidak membocorkan informasi apakah code itu exist atau bukan.
func DeleteLink(c *gin.Context) {
	code := c.Param("code")
	userID, _ := c.Get("user_id")

	var linkID int
	var ownerID int
	err := config.DB.QueryRow(
		`SELECT id, user_id FROM links WHERE code = $1`, code,
	).Scan(&linkID, &ownerID)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Link tidak ditemukan"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data link"})
		return
	}

	if ownerID != userID.(int) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Link tidak ditemukan"})
		return
	}

	tx, err := config.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memulai transaksi"})
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM link_visits WHERE link_id = $1`, linkID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghapus history klik"})
		return
	}

	if _, err := tx.Exec(`DELETE FROM links WHERE id = $1`, linkID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghapus link"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghapus link"})
		return
	}

	// Hapus juga dari cache Redis supaya tidak masih bisa redirect dari cache lama
	config.RDB.Del(config.Ctx, "link:"+code)

	c.JSON(http.StatusOK, gin.H{"message": "Link berhasil dihapus"})
}
