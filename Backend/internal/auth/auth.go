package auth

import (
	"context"
	"net/http"
	"strings"

	"firebase.google.com/go/v4/auth"
)

type FirebaseAuthMiddleware struct {
	authClient *auth.Client
}

func NewFirebaseAuthMiddleware(authClient *auth.Client) *FirebaseAuthMiddleware {
	return &FirebaseAuthMiddleware{authClient: authClient}
}

func (m *FirebaseAuthMiddleware) Authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		idToken := strings.TrimSpace(strings.Replace(authHeader, "Bearer", "", 1))

		if idToken == "" {
			http.Error(w, "No token provided", http.StatusUnauthorized)
			return
		}

		token, err := m.authClient.VerifyIDToken(r.Context(), idToken)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "user", token)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
