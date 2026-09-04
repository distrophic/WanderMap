package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type Suggestion struct {
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name"`
	City        string  `json:"city"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

type Client struct {
	http      *http.Client
	baseURL   string
	userAgent string

	mu   sync.Mutex
	last time.Time
}

func NewClient() *Client {
	return &Client{
		http:      &http.Client{Timeout: 10 * time.Second},
		baseURL:   "https://nominatim.openstreetmap.org",
		userAgent: "TravelGuide/1.0 (personal desktop app)",
	}
}

// Suggest выполняет поиск мест. Запросы сериализованы мьютексом
// с паузой в 1 секунду — политика Nominatim запрещает чаще.
// Для одного пользователя за клавиатурой это незаметно.
func (c *Client) Suggest(ctx context.Context, query string, limit int) ([]Suggestion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if wait := time.Second - time.Since(c.last); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c.last = time.Now()

	if limit <= 0 || limit > 10 {
		limit = 5
	}

	q := url.Values{
		"q":               {query},
		"format":          {"jsonv2"},
		"addressdetails":  {"1"},
		"limit":           {strconv.Itoa(limit)},
		"accept-language": {"ru,en"}, // русские названия, если есть; иначе английские
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/search?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent) // обязателен по политике Nominatim

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nominatim: статус %d", resp.StatusCode)
	}

	var raw []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Lat         string `json:"lat"` // Nominatim отдаёт координаты строками
		Lon         string `json:"lon"`
		Address     struct {
			City         string `json:"city"`
			Town         string `json:"town"`
			Village      string `json:"village"`
			Municipality string `json:"municipality"`
			Country      string `json:"country"`
			CountryCode  string `json:"country_code"`
		} `json:"address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("nominatim: разбор ответа: %w", err)
	}

	out := make([]Suggestion, 0, len(raw))
	for _, r := range raw {
		lat, _ := strconv.ParseFloat(r.Lat, 64)
		lon, _ := strconv.ParseFloat(r.Lon, 64)

		city := firstNonEmpty(r.Address.City, r.Address.Town,
			r.Address.Village, r.Address.Municipality)

		out = append(out, Suggestion{
			Name:        r.Name,
			DisplayName: r.DisplayName,
			City:        city,
			Country:     r.Address.Country,
			CountryCode: r.Address.CountryCode,
			Latitude:    lat,
			Longitude:   lon,
		})
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}