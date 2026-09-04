package api

import (
	"net/http"
	"strings"
)

// GET /api/geocode?q=лувр → подсказки Nominatim для формы добавления.
// UI должен дебаунсить ввод (300–500 мс после последнего символа),
// иначе упрёмся в лимит 1 запрос/сек.
func (s *server) geocode(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 3 { // руны, не байты: "лу" — это 4 байта, но 2 символа
		writeError(w, http.StatusBadRequest, "запрос должен содержать минимум 3 символа")
		return
	}

	suggestions, err := s.geo.Suggest(r.Context(), q, 5)
	if err != nil {
		writeError(w, http.StatusBadGateway, "сервис геокодирования недоступен")
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}