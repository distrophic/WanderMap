package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"travelguide/internal/store"
)

// GET /api/places?search=&country=&category=&favorites=1&famous=1
// Все параметры опциональны и комбинируются.
func (s *server) listPlaces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.Filter{
		Search:        q.Get("search"),
		Country:       q.Get("country"),
		Category:      q.Get("category"),
		OnlyFavorites: isTruthy(q.Get("favorites")),
		OnlyFamous:    isTruthy(q.Get("famous")),
	}

	places, err := s.repo.List(r.Context(), f)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, places)
}

// POST /api/places → 201 + объект с присвоенным id.
func (s *server) createPlace(w http.ResponseWriter, r *http.Request) {
	p, ok := decodePlace(w, r)
	if !ok {
		return
	}
	if err := s.repo.Insert(r.Context(), &p); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// PUT /api/places/{id} → 200 + обновлённый объект, либо 404.
func (s *server) updatePlace(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, ok := decodePlace(w, r)
	if !ok {
		return
	}
	p.ID = id // id берётся из пути; значение в теле игнорируется

	if err := s.repo.Update(r.Context(), &p); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// DELETE /api/places/{id} → 204, либо 404.
func (s *server) deletePlace(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.repo.Delete(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/places/{id}/toggle-favorite → {"is_favorite": true}
func (s *server) toggleFavorite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	val, err := s.repo.ToggleFavorite(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"is_favorite": val})
}

// POST /api/places/{id}/toggle-famous → {"is_famous": false}
func (s *server) toggleFamous(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	val, err := s.repo.ToggleFamous(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"is_famous": val})
}

// GET /api/countries → ["Италия", "Франция", ...] для выпадашки.
func (s *server) countries(w http.ResponseWriter, r *http.Request) {
	countries, err := s.repo.Countries(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, countries)
}

// ── помощники ──────────────────────────────────

func decodePlace(w http.ResponseWriter, r *http.Request) (store.Place, bool) {
	var p store.Place
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 МБ хватит любому месту

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // опечатка в имени поля = ошибка, а не тихая потеря данных
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON: "+err.Error())
		return p, false
	}
	if p.Name == "" {
		writeError(w, http.StatusUnprocessableEntity, "поле name обязательно")
		return p, false
	}
	if p.Latitude < -90 || p.Latitude > 90 || p.Longitude < -180 || p.Longitude > 180 {
		writeError(w, http.StatusUnprocessableEntity, "координаты вне допустимого диапазона")
		return p, false
	}
	return p, true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "некорректный id")
		return 0, false
	}
	return id, true
}

func isTruthy(s string) bool {
	return s == "1" || s == "true"
}