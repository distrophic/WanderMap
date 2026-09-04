package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("place not found")

type Place struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	City        string   `json:"city"`
	Country     string   `json:"country"`
	Description string   `json:"description"`
	Latitude    float64  `json:"latitude"`
	Longitude   float64  `json:"longitude"`
	IsFavorite  bool     `json:"is_favorite"`
	IsFamous    bool     `json:"is_famous"`
	ImageURLs   []string `json:"image_urls"`
}

type Filter struct {
	Search        string
	Country       string
	Category      string
	OnlyFavorites bool
	OnlyFamous    bool
}

type Repo struct {
	db *sql.DB
}

func Open(path string) (*Repo, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite — один писатель. Для desktop-масштаба проще сериализовать всё
	// и забыть про SQLITE_BUSY, чем жонглировать пулом.
	db.SetMaxOpenConns(1)

	r := &Repo{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return r, nil
}

func (r *Repo) Close() error { return r.db.Close() }

// Имена совпадают с C#-версией — Go-бэкенд открывает старый .db как есть.
func (r *Repo) migrate() error {
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS Places (
		Id          INTEGER PRIMARY KEY AUTOINCREMENT,
		Name        TEXT NOT NULL,
		Category    TEXT NOT NULL DEFAULT 'Другое',
		City        TEXT NOT NULL DEFAULT '',
		Country     TEXT NOT NULL DEFAULT '',
		Description TEXT NOT NULL DEFAULT '',
		Latitude    REAL NOT NULL DEFAULT 0,
		Longitude   REAL NOT NULL DEFAULT 0,
		IsFavorite  INTEGER NOT NULL DEFAULT 0,
		IsFamous    INTEGER NOT NULL DEFAULT 0,
		ImageUrls   TEXT NOT NULL DEFAULT '[]'
	)`)
	if err != nil {
		return err
	}
	// Замена try/catch вокруг GetOrdinal: миграция один раз при старте,
	// а не на каждой прочитанной строке.
	return r.ensureColumn("IsFamous", "INTEGER NOT NULL DEFAULT 0")
}

func (r *Repo) ensureColumn(name, decl string) error {
	rows, err := r.db.Query(`PRAGMA table_info(Places)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var colName, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = r.db.Exec(fmt.Sprintf(`ALTER TABLE Places ADD COLUMN %s %s`, name, decl))
	return err
}

// ══════════════════════════════════════════════
//  ЧТЕНИЕ
// ══════════════════════════════════════════════

// List — единственный метод чтения списка: GetAll, Search, GetByCategory,
// GetByCountry, GetFavorites, GetFamous и GetFiltered в одном.
//
// Разделение труда: категория и флаги фильтруются в SQL (точные значения
// из нашего же кода), а поиск и страна — в Go, потому что LIKE и NOCASE
// в SQLite не понимают регистр кириллицы.
func (r *Repo) List(ctx context.Context, f Filter) ([]Place, error) {
	where := "1=1"
	var args []any

	if f.Category != "" {
		where += " AND Category = ?"
		args = append(args, f.Category)
	}
	if f.OnlyFavorites {
		where += " AND IsFavorite = 1"
	}
	if f.OnlyFamous {
		where += " AND IsFamous = 1"
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT Id, Name, Category, City, Country, Description,
		        Latitude, Longitude, IsFavorite, IsFamous, ImageUrls
		 FROM Places WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	search := strings.ToLower(strings.TrimSpace(f.Search))
	country := strings.ToLower(strings.TrimSpace(f.Country))

	places := []Place{}
	for rows.Next() {
		p, err := scanPlace(rows)
		if err != nil {
			return nil, err
		}
		if country != "" && strings.ToLower(strings.TrimSpace(p.Country)) != country {
			continue
		}
		if search != "" && !matchesSearch(p, search) {
			continue
		}
		places = append(places, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// ORDER BY Name в SQLite сортирует кириллицу побайтово: все заглавные
	// раньше всех строчных. Сортируем здесь, без учёта регистра.
	sort.Slice(places, func(i, j int) bool {
		return strings.ToLower(places[i].Name) < strings.ToLower(places[j].Name)
	})
	return places, nil
}

func matchesSearch(p Place, q string) bool {
	// Description в списке = поиск «Франция» зацепит все места, где она
	// упомянута в вики-тексте. Если это шум — убери p.Description.
	for _, field := range []string{p.Name, p.City, p.Country, p.Description, p.Category} {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	return false
}

// Countries возвращает список стран для фильтра-выпадашки.
// Варианты написания, различающиеся регистром или пробелами,
// схлопываются; каноническим считается самое частое написание.
func (r *Repo) Countries(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT TRIM(Country), COUNT(*) FROM Places
		 WHERE TRIM(Country) != '' GROUP BY TRIM(Country)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type variant struct {
		spelling string
		count    int
	}
	best := map[string]variant{} // ключ — lower(страна)

	for rows.Next() {
		var name string
		var cnt int
		if err := rows.Scan(&name, &cnt); err != nil {
			return nil, err
		}
		key := strings.ToLower(name)
		if v, ok := best[key]; !ok || cnt > v.count {
			best[key] = variant{name, cnt}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(best))
	for _, v := range best {
		out = append(out, v.spelling)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out, nil
}

// ══════════════════════════════════════════════
//  ИЗМЕНЕНИЕ
// ══════════════════════════════════════════════

func (r *Repo) Insert(ctx context.Context, p *Place) error {
	normalize(p)
	urls, _ := json.Marshal(p.ImageURLs)
	// RETURNING: UI сразу получает Id, без перезагрузки списка.
	return r.db.QueryRowContext(ctx, `
		INSERT INTO Places (Name, Category, City, Country, Description,
		                    Latitude, Longitude, IsFavorite, IsFamous, ImageUrls)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING Id`,
		p.Name, p.Category, p.City, p.Country, p.Description,
		p.Latitude, p.Longitude, b2i(p.IsFavorite), b2i(p.IsFamous), string(urls),
	).Scan(&p.ID)
}

func (r *Repo) Update(ctx context.Context, p *Place) error {
	normalize(p)
	urls, _ := json.Marshal(p.ImageURLs)
	res, err := r.db.ExecContext(ctx, `
		UPDATE Places SET
			Name = ?, Category = ?, City = ?, Country = ?, Description = ?,
			Latitude = ?, Longitude = ?, IsFavorite = ?, IsFamous = ?, ImageUrls = ?
		WHERE Id = ?`,
		p.Name, p.Category, p.City, p.Country, p.Description,
		p.Latitude, p.Longitude, b2i(p.IsFavorite), b2i(p.IsFamous), string(urls),
		p.ID)
	if err != nil {
		return err
	}
	return checkAffected(res)
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM Places WHERE Id = ?`, id)
	if err != nil {
		return err
	}
	return checkAffected(res)
}

// ToggleFavorite возвращает новое состояние — UI обновляет звёздочку
// без перечитывания записи.
func (r *Repo) ToggleFavorite(ctx context.Context, id int64) (bool, error) {
	return r.toggle(ctx, "IsFavorite", id)
}

func (r *Repo) ToggleFamous(ctx context.Context, id int64) (bool, error) {
	return r.toggle(ctx, "IsFamous", id)
}

func (r *Repo) toggle(ctx context.Context, col string, id int64) (bool, error) {
	var v int
	err := r.db.QueryRowContext(ctx, fmt.Sprintf(
		`UPDATE Places SET %s = 1 - %s WHERE Id = ? RETURNING %s`, col, col, col),
		id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	return v == 1, err
}

// ══════════════════════════════════════════════
//  ВСПОМОГАТЕЛЬНЫЕ
// ══════════════════════════════════════════════

// normalize подрезает пробелы на записи — половина лекарства
// от дублей стран вида "Франция " / "Франция".
func normalize(p *Place) {
	p.Name = strings.TrimSpace(p.Name)
	p.City = strings.TrimSpace(p.City)
	p.Country = strings.TrimSpace(p.Country)
	p.Category = strings.TrimSpace(p.Category)
	if p.Category == "" {
		p.Category = "Другое"
	}
	if p.ImageURLs == nil {
		p.ImageURLs = []string{} // иначе json.Marshal даст "null", а не "[]"
	}
}

func scanPlace(rows *sql.Rows) (Place, error) {
	var p Place
	var fav, fam int
	var urls sql.NullString
	var city, country, desc sql.NullString
	var lat, lon sql.NullFloat64

	err := rows.Scan(&p.ID, &p.Name, &p.Category, &city, &country, &desc,
		&lat, &lon, &fav, &fam, &urls)
	if err != nil {
		return p, err
	}
	p.City, p.Country, p.Description = city.String, country.String, desc.String
	p.Latitude, p.Longitude = lat.Float64, lon.Float64
	p.IsFavorite, p.IsFamous = fav == 1, fam == 1

	p.ImageURLs = []string{}
	if urls.Valid && urls.String != "" {
		_ = json.Unmarshal([]byte(urls.String), &p.ImageURLs) // битый JSON → пустой список
	}
	return p, nil
}

func checkAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}