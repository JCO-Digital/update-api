package api

import (
	"crypto/subtle"
	"net/http"
)

func AdminAuthMiddleware(adminKeys []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				http.Error(w, "Unauthorized: Missing API Key", http.StatusUnauthorized)
				return
			}

			authorized := false
			for _, key := range adminKeys {
				if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) == 1 {
					authorized = true
				}
			}

			if !authorized {
				http.Error(w, "Unauthorized: Invalid API Key", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
