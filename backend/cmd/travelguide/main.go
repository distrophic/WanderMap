package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"travelguide/internal/api"
	"travelguide/internal/geo"
	"travelguide/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "адрес HTTP-сервера")
	dbPath := flag.String("db", defaultDBPath(), "путь к файлу базы данных")
	flag.Parse()

	repo, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("не удалось открыть БД %s: %v", *dbPath, err)
	}
	defer repo.Close()
	log.Printf("база данных: %s", *dbPath)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(repo, geo.NewClient()),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown: Ctrl+C или SIGTERM (от systemd — пригодится
	// на этапе Linux-оптимизации).
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("сервер запущен: http://%s", *addr)
		if err := srv.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("сервер: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("останавливаюсь...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// defaultDBPath кладёт базу в стандартный каталог конфигурации:
// на Linux это ~/.config/travelguide/places.db (XDG).
// Флагом -db можно указать старый .db-файл от C#-версии —
// схема совместима, миграция подхватит его как есть.
func defaultDBPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "places.db"
	}
	appDir := filepath.Join(dir, "travelguide")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "places.db"
	}
	return filepath.Join(appDir, "places.db")
}