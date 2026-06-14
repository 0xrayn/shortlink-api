package validators

import (
	"net/url"
	"regexp"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var linkCodePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// validateLinkCode memastikan custom alias hanya berisi huruf, angka, strip, dan underscore.
// Ini penting karena code dipakai langsung sebagai bagian dari URL path.
func validateLinkCode(fl validator.FieldLevel) bool {
	return linkCodePattern.MatchString(fl.Field().String())
}

// validateHTTPURL memastikan URL valid dan scheme-nya http atau https.
// Mencegah penyalahgunaan scheme lain seperti javascript:, ftp:, file:, dll.
func validateHTTPURL(fl validator.FieldLevel) bool {
	raw := fl.Field().String()
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	return true
}

// RegisterCustomValidators mendaftarkan validator custom ke Gin's validator engine.
// Dipanggil sekali saat startup, sebelum routes didaftarkan.
func RegisterCustomValidators() error {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return nil
	}

	if err := v.RegisterValidation("linkcode", validateLinkCode); err != nil {
		return err
	}
	if err := v.RegisterValidation("http_url", validateHTTPURL); err != nil {
		return err
	}
	return nil
}
