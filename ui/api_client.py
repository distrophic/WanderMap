"""HTTP-клиент к Go-бэкенду. Единственный файл UI, знающий про сеть."""

from dataclasses import dataclass, field, asdict

import requests

BASE_URL = "http://127.0.0.1:8080"


class ApiError(Exception):
    """Любая ошибка общения с бэкендом."""


class NotFoundError(ApiError):
    """Бэкенд ответил 404 — место не существует (ErrNotFound на стороне Go)."""


@dataclass
class Place:
    id: int = 0
    name: str = ""
    category: str = "Другое"
    city: str = ""
    country: str = ""
    description: str = ""
    latitude: float = 0.0
    longitude: float = 0.0
    is_favorite: bool = False
    is_famous: bool = False
    image_urls: list = field(default_factory=list)

    # Имена полей совпадают с JSON-тегами Go-структуры, поэтому
    # сериализация в обе стороны — тривиальная.
    @classmethod
    def from_json(cls, d: dict) -> "Place":
        return cls(**d)

    def to_json(self) -> dict:
        return asdict(self)


class ApiClient:
    def __init__(self, base_url: str = BASE_URL):
        self.base = base_url.rstrip("/")
        self.http = requests.Session()

    # ── внутреннее ──────────────────────────────

    def _request(self, method: str, path: str, **kwargs):
        try:
            resp = self.http.request(method, self.base + path, timeout=15, **kwargs)
        except requests.RequestException as e:
            raise ApiError(f"бэкенд недоступен: {e}") from e

        if resp.status_code == 404:
            raise NotFoundError("место не найдено")
        if resp.status_code >= 400:
            try:
                msg = resp.json().get("error", resp.text)
            except ValueError:
                msg = resp.text
            raise ApiError(msg)
        if resp.status_code == 204:
            return None
        return resp.json()

    # ── места ───────────────────────────────────

    def list_places(self, search: str = "", country: str = "", category: str = "",
                    favorites: bool = False, famous: bool = False) -> list[Place]:
        params = {}
        if search:
            params["search"] = search
        if country:
            params["country"] = country
        if category:
            params["category"] = category
        if favorites:
            params["favorites"] = "1"
        if famous:
            params["famous"] = "1"
        data = self._request("GET", "/api/places", params=params)
        return [Place.from_json(d) for d in data]

    def create(self, place: Place) -> Place:
        # Бэкенд отвечает 201 + объект с присвоенным id (RETURNING в Go) —
        # перезагружать весь список ради нового id не нужно.
        data = self._request("POST", "/api/places", json=place.to_json())
        return Place.from_json(data)

    def update(self, place: Place) -> Place:
        data = self._request("PUT", f"/api/places/{place.id}", json=place.to_json())
        return Place.from_json(data)

    def delete(self, place_id: int) -> None:
        self._request("DELETE", f"/api/places/{place_id}")

    def toggle_favorite(self, place_id: int) -> bool:
        """Возвращает НОВОЕ состояние — UI обновляет звёздочку без перезапроса."""
        data = self._request("POST", f"/api/places/{place_id}/toggle-favorite")
        return data["is_favorite"]

    def toggle_famous(self, place_id: int) -> bool:
        data = self._request("POST", f"/api/places/{place_id}/toggle-famous")
        return data["is_famous"]

    # ── справочники ─────────────────────────────

    def countries(self) -> list[str]:
        return self._request("GET", "/api/countries")

    def geocode(self, query: str) -> list[dict]:
        return self._request("GET", "/api/geocode", params={"q": query})