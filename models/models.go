package models

import "time"

type User struct {
	ID           int       `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Link struct {
	ID          int       `json:"id"`
	Code        string    `json:"code"`
	OriginalURL string    `json:"original_url"`
	UserID      int       `json:"user_id"`
	ClickCount  int64     `json:"click_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type LinkVisit struct {
	ID        int       `json:"id"`
	LinkID    int       `json:"link_id"`
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"ip_address"`
	VisitedAt time.Time `json:"visited_at"`
}

// Request payloads

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type CreateLinkRequest struct {
	URL  string `json:"url" binding:"required,url"`
	Code string `json:"code"` // optional custom alias
}
