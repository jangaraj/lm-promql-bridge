package httpauth

import (
	"crypto/subtle"
	"net/http"
)

func BasicAuth(username, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		gotUser, gotPass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(gotUser), []byte(username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(gotPass), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="LM PromQL Bridge"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
