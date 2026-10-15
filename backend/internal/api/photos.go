package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const maxPhotoBytes = 20 << 20 // 20 МБ

// POST /api/places/{id}/photos, поле формы file.
// Файл ложится в каталог рядом с базой. В image_urls пишется только
// относительный путь вида photos/имя.jpg — не байты и не внешний URL.
func (s *server) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.repo.Get(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "нужен файл в поле file, не больше 20 МБ")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "поле file обязательно")
		return
	}
	defer file.Close()

	header := make([]byte, 512)
	n, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "не удалось прочитать файл")
		return
	}
	ext, ok := photoExt(header[:n])
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "нужна фотография JPEG, PNG, GIF или WebP")
		return
	}

	name := fmt.Sprintf("%d-%d%s", id, time.Now().UnixNano(), ext)
	full := filepath.Join(s.photosDir, name)
	dst, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		s.fail(w, err)
		return
	}

	if _, err := dst.Write(header[:n]); err != nil {
		dst.Close()
		os.Remove(full)
		s.fail(w, err)
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		os.Remove(full)
		writeError(w, http.StatusBadRequest, "не удалось сохранить файл")
		return
	}
	if err := dst.Close(); err != nil {
		os.Remove(full)
		s.fail(w, err)
		return
	}

	rel := path.Join("photos", name) // слэши как в URL, не как в Windows
	place, err := s.repo.AddImage(r.Context(), id, rel)
	if err != nil {
		os.Remove(full)
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, place)
}

// GET /api/photos/{name} отдаёт файл из каталога фотографий.
// Имя — только база пути: photos/1-2.jpg из базы запрашивается как /api/photos/1-2.jpg.
func (s *server) servePhoto(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" || name != path.Base(name) || strings.Contains(name, "..") {
		writeError(w, http.StatusBadRequest, "некорректное имя файла")
		return
	}

	full := filepath.Join(s.photosDir, name)
	f, err := os.Open(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "файл не найден")
			return
		}
		s.fail(w, err)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		writeError(w, http.StatusNotFound, "файл не найден")
		return
	}
	http.ServeContent(w, r, name, stat.ModTime(), f)
}

func photoExt(head []byte) (string, bool) {
	if len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP" {
		return ".webp", true
	}
	switch http.DetectContentType(head) {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/gif":
		return ".gif", true
	default:
		return "", false
	}
}
