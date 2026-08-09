package server

import "net/http"

// New creates the minimal HTTP server for the Go API.
func New() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)

	return &http.Server{Handler: mux}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("okhhhh\n"))
}
