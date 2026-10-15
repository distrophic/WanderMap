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

	photosDir := filepath.Join(filepath.Dir(*dbPath), "photos")
	if err := os.MkdirAll(photosDir, 0o755); err != nil {
		log.Fatalf("не удалось создать каталог фотографий %s: %v", photosDir, err)
	}
	log.Printf("фотографии: %s", photosDir)

	webDir := findWebDir()
	if webDir == "" {
		log.Println("веб-страница не найдена: откройте ui/web/index.html отдельно, API работает")
	} else {
		log.Printf("веб-страница: %s", webDir)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(repo, geo.NewClient(), photosDir, webDir),
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
// findWebDir ищет ui/web и рядом с бинарником, и от текущего каталога.
// Окно Qt по-прежнему открывает map.html само; index.html — страница в браузере.
func findWebDir() string {
	candidates := []string{
		filepath.Join("ui", "web"),
		filepath.Join("..", "ui", "web"),
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		candidates = append([]string{
			filepath.Join(base, "..", "ui", "web"),
			filepath.Join(base, "ui", "web"),
		}, candidates...)
	}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(filepath.Join(abs, "index.html"))
		if err == nil && !info.IsDir() {
			return abs
		}
	}
	return ""
}

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