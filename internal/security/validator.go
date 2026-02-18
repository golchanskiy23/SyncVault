package security

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
)

// emailRegex is a basic RFC 5322-ish email pattern.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Validator provides input validation helpers.
// Bug 1.13 fix: security.NewValidator() was referenced in cmd/auth-service/main.go
// but did not exist in the package.
type Validator struct{}

// NewValidator creates a new Validator.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateStruct validates exported fields of a struct using `validate` struct tags.
// Supported tags: required, email, min=N (string length).
// Example: `validate:"required,email"` or `validate:"required,min=8"`
func (v *Validator) ValidateStruct(s interface{}) error {
	if s == nil {
		return fmt.Errorf("validation target must not be nil")
	}

	val := reflect.ValueOf(s)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return fmt.Errorf("validation target must not be nil")
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil
	}

	t := val.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fval := val.Field(i)
		tag := field.Tag.Get("validate")
		if tag == "" {
			continue
		}

		strVal := fmt.Sprintf("%v", fval.Interface())
		rules := strings.Split(tag, ",")

		for _, rule := range rules {
			rule = strings.TrimSpace(rule)
			switch {
			case rule == "required":
				if strVal == "" {
					return fmt.Errorf("field %s is required", field.Name)
				}
			case rule == "email":
				if !emailRegex.MatchString(strVal) {
					return fmt.Errorf("field %s must be a valid email address", field.Name)
				}
			case strings.HasPrefix(rule, "min="):
				var minLen int
				fmt.Sscanf(rule, "min=%d", &minLen)
				if len(strVal) < minLen {
					return fmt.Errorf("field %s must be at least %d characters", field.Name, minLen)
				}
			}
		}
	}

	return nil
}

// SecurityHeadersMiddleware adds common security headers to every response.
// Bug 1.13 fix: security.SecurityHeadersMiddleware was referenced but did not exist.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

		// Remove server fingerprinting headers
		w.Header().Del("Server")
		w.Header().Del("X-Powered-By")

		// Allow CORS for API clients
		origin := r.Header.Get("Origin")
		if origin != "" && isTrustedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isTrustedOrigin(origin string) bool {
	trusted := []string{"http://localhost", "https://localhost"}
	for _, t := range trusted {
		if strings.HasPrefix(origin, t) {
			return true
		}
	}
	return false
}
