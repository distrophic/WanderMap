package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"travelguide/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("кодирование ответа: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// fail переводит ошибки хранилища в HTTP-статусы.
// Детали внутренних ошибок остаются в логе, наружу уходит общий текст.
func (s *server) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "место не найдено")
		return
	}
	log.Printf("внутренняя ошибка: %v", err)
	writeError(w, http.StatusInternalServerError, "внутренняя ошибка сервера")
}