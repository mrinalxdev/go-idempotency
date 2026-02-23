package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

func HMACAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sig := r.Header.Get("X-Signature")
			if sig == "" {
				http.Error(w, "X-Signature header required", http.StatusUnauthorized)
				return
			}

			payload := r.Header.Get("Idempotency-Key") + r.Header.Get("Content-Type")
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(payload))
			expected := hex.EncodeToString(mac.Sum(nil))

			if !hmac.Equal([]byte(sig), []byte(expected)) {
				http.Error(w, "Invalid signature", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}