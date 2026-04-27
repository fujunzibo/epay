package app

import (
	"net/http"
	"strings"
)

// Handler returns the HTTP handler with CORS applied for browser admin UI.
func (a *App) Handler() http.Handler {
	return a.withCORS(a.mux)
}

func (a *App) withCORS(next http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	raw := strings.TrimSpace(a.cfg.CORSAllowOrigins)
	if raw == "" {
		raw = "http://localhost:5173,http://127.0.0.1:5173"
	}
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed[o] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Stripe-Signature, X-USDC-Signature")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
