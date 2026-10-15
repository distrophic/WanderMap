package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"travelguide/internal/geo"
	"travelguide/internal/store"
)

func TestPhotoSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "places.db")
	photos := filepath.Join(dir, "photos")
	if err := os.MkdirAll(photos, 0o755); err != nil {
		t.Fatal(err)
	}

	repo, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	place := store.Place{Name: "Лувр"}
	if err := repo.Insert(context.Background(), &place); err != nil {
		t.Fatal(err)
	}
	handler := New(repo, geo.NewClient(), photos, "")

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "shot.png")
	if err != nil {
		t.Fatal(err)
	}
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0, 0, 0, 0, 0, 0, 0, 0,
	}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	form.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/places/1/photos", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}

	var saved store.Place
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.ImageURLs) != 1 || !bytes.HasPrefix([]byte(saved.ImageURLs[0]), []byte("photos/")) {
		t.Fatalf("path: %#v", saved.ImageURLs)
	}

	repo.Close()

	repo, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	handler = New(repo, geo.NewClient(), photos, "")

	got, err := repo.Get(context.Background(), place.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ImageURLs) != 1 || got.ImageURLs[0] != saved.ImageURLs[0] {
		t.Fatalf("after reopen: %#v", got.ImageURLs)
	}

	name := filepath.Base(got.ImageURLs[0])
	get := httptest.NewRequest(http.MethodGet, "/api/photos/"+name, nil)
	get.SetPathValue("name", name)
	gotRec := httptest.NewRecorder()
	handler.ServeHTTP(gotRec, get)
	if gotRec.Code != http.StatusOK {
		t.Fatalf("get photo: %d", gotRec.Code)
	}
	data, _ := io.ReadAll(gotRec.Body)
	if !bytes.Equal(data, png) {
		t.Fatalf("bytes differ: %d vs %d", len(data), len(png))
	}
}
