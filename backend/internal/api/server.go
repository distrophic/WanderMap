package api

import (
	"log"
	"net/http"
	"time"

	"travelguide/internal/geo"
	"travelguide/internal/store"
)

type server struct {
	repo *store.Repo
	geo  *geo.Client
}

func New(repo *store.Repo, geocoder *geo.Client) http.Handler {
	s := &server{repo: repo, geo: geocoder}

	// Роутинг стандартной библиотеки (Go 1.22+): методы и {id} в путях
	// работают из коробки, сторонний роутер не нужен.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/places", s.listPlaces)
	mux.HandleFunc("POST /api/places", s.createPlace)
	mux.HandleFunc("PUT /api/places/{id}", s.updatePlace)
	mux.HandleFunc("DELETE /api/places/{id}", s.deletePlace)
	mux.HandleFunc("POST /api/places/{id}/toggle-favorite", s.toggleFavorite)
	mux.HandleFunc("POST /api/places/{id}/toggle-famous", s.toggleFamous)
	mux.HandleFunc("GET /api/countries", s.countries)
	mux.HandleFunc("GET /api/geocode", s.geocode)

	return logMiddleware(corsMiddleware(mux))
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// corsMiddleware нужен только если UI — веб-страница в браузере.
// Desktop-клиент (Qt и т.п.) ходит на localhost напрямую, ему CORS
// безразличен. Сервер слушает 127.0.0.1, поэтому "*" здесь безопасен.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}