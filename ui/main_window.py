"""Главное окно: фильтры, список мест, карта, описание."""

import json
from pathlib import Path

from PyQt6.QtCore import Qt, QTimer, QUrl, QObject, pyqtSignal, pyqtSlot
from PyQt6.QtWebChannel import QWebChannel
from PyQt6.QtWebEngineCore import QWebEngineSettings
from PyQt6.QtWebEngineWidgets import QWebEngineView
from PyQt6.QtWidgets import (
    QCheckBox, QComboBox, QHBoxLayout, QLineEdit, QListWidget, QListWidgetItem,
    QMainWindow, QMessageBox, QPushButton, QSplitter, QTextBrowser,
    QVBoxLayout, QWidget,
)

from constants import CATEGORIES
from api_client import ApiClient, ApiError, NotFoundError, Place
from place_dialog import PlaceDialog


ALL_COUNTRIES = "Все страны"
ALL_CATEGORIES = "Все категории"


class MapBridge(QObject):
    """Мост Qt ↔ JS: клик по маркеру на карте прилетает сюда."""
    marker_clicked = pyqtSignal(int)

    @pyqtSlot(int)
    def onMarkerClicked(self, place_id: int):
        self.marker_clicked.emit(place_id)


class MainWindow(QMainWindow):
    def __init__(self, api: ApiClient):
        super().__init__()
        self.api = api
        self.places: list[Place] = []
        self._map_ready = False

        self.setWindowTitle("WanderMap")
        self.resize(1100, 700)
        self._build_ui()
        self._reload_countries()
        self.refresh()

    # ── интерфейс ───────────────────────────────

    def _build_ui(self):
        # Фильтры
        self.search = QLineEdit(placeholderText="Поиск: название, город, страна…")
        self.search.setObjectName("searchField")

        self.country_box = QComboBox()
        self.country_box.setObjectName("categoryFilter")

        self.category_box = QComboBox()
        self.category_box.setObjectName("categoryFilter")
        self.category_box.addItem(ALL_CATEGORIES)
        self.category_box.addItems(CATEGORIES)
        self.fav_check = QCheckBox("⭐ Избранные")
        self.famous_check = QCheckBox("🏛 Знаменитые")

        # Дебаунс поиска: 400 мс после последнего символа — та же пауза,
        # что мы договаривались держать для геокодера.
        self._debounce = QTimer(self, singleShot=True, interval=400)
        self._debounce.timeout.connect(self.refresh)
        self.search.textChanged.connect(self._debounce.start)

        # Остальные фильтры срабатывают сразу
        self.country_box.currentIndexChanged.connect(self.refresh)
        self.category_box.currentIndexChanged.connect(self.refresh)
        self.fav_check.toggled.connect(self.refresh)
        self.famous_check.toggled.connect(self.refresh)

        filters = QHBoxLayout()
        filters.addWidget(self.search, stretch=2)
        filters.addWidget(self.country_box, stretch=1)
        filters.addWidget(self.category_box, stretch=1)
        filters.addWidget(self.fav_check)
        filters.addWidget(self.famous_check)

        # Список и кнопки
        self.list = QListWidget()
        self.list.setObjectName("placesList") 
        self.list.currentRowChanged.connect(self._on_select)
        self.list.itemDoubleClicked.connect(lambda _: self._edit())

        btn_add = QPushButton("Добавить")
        btn_add.setObjectName("addButton")
        btn_edit = QPushButton("Изменить")
        btn_del = QPushButton("Удалить")
        btn_fav = QPushButton("⭐")
        btn_famous = QPushButton("🏛")
        btn_add.clicked.connect(self._add)
        btn_edit.clicked.connect(self._edit)
        btn_del.clicked.connect(self._delete)
        btn_fav.clicked.connect(self._toggle_favorite)
        btn_famous.clicked.connect(self._toggle_famous)

        buttons = QHBoxLayout()
        for b in (btn_add, btn_edit, btn_del, btn_fav, btn_famous):
            buttons.addWidget(b)

        left = QVBoxLayout()
        left.addWidget(self.list)
        left.addLayout(buttons)
        left_widget = QWidget()
        left_widget.setLayout(left)

        # Карта (JS/Leaflet в QWebEngineView) + описание
        self.map_view = QWebEngineView()
        # Без этого QtWebEngine блокирует запросы file://-страницы к внешним URL
        # (тайлы OpenStreetMap). Убрать, если map.html переедет на http://127.0.0.1:8080.
        self.map_view.settings().setAttribute(QWebEngineSettings.WebAttribute.LocalContentCanAccessRemoteUrls, True)
        self.bridge = MapBridge()
        self.bridge.marker_clicked.connect(self._select_by_id)
        channel = QWebChannel(self.map_view.page())
        channel.registerObject("bridge", self.bridge)
        self.map_view.page().setWebChannel(channel)
        map_html = Path(__file__).parent / "web" / "map.html"
        self.map_view.load(QUrl.fromLocalFile(str(map_html)))
        self.map_view.loadFinished.connect(self._on_map_loaded)

        self.description = QTextBrowser()
        self.description.setStyleSheet("background: #ffffff; color: #1a1a1a;")
        self.description.setMaximumHeight(160)

        right = QVBoxLayout()
        right.addWidget(self.map_view, stretch=1)
        right.addWidget(self.description)
        right_widget = QWidget()
        right_widget.setLayout(right)

        splitter = QSplitter(Qt.Orientation.Horizontal)
        splitter.addWidget(left_widget)
        splitter.addWidget(right_widget)
        splitter.setSizes([420, 680])

        root = QVBoxLayout()
        root.addLayout(filters)
        root.addWidget(splitter, stretch=1)
        central = QWidget()
        central.setLayout(root)
        self.setCentralWidget(central)

    # ── данные ──────────────────────────────────

    def refresh(self):
        country = self.country_box.currentText()
        category = self.category_box.currentText()
        try:
            # Вся фильтрация — на бэкенде. Кириллический регистр
            # там уже решён, UI ничего не изобретает.
            self.places = self.api.list_places(
                search=self.search.text(),
                country="" if country == ALL_COUNTRIES else country,
                category="" if category == ALL_CATEGORIES else category,
                favorites=self.fav_check.isChecked(),
                famous=self.famous_check.isChecked(),
            )
        except ApiError as e:
            QMessageBox.warning(self, "TravelGuide", str(e))
            return

        self.list.clear()
        for p in self.places:
            self.list.addItem(QListWidgetItem(self._label(p)))
        self._push_to_map()

    def _reload_countries(self):
        """Наполняет фильтр стран; вызывается при старте и после изменений."""
        current = self.country_box.currentText()
        try:
            countries = self.api.countries()
        except ApiError:
            countries = []
        self.country_box.blockSignals(True)
        self.country_box.clear()
        self.country_box.addItem(ALL_COUNTRIES)
        self.country_box.addItems(countries)
        idx = self.country_box.findText(current)
        self.country_box.setCurrentIndex(idx if idx >= 0 else 0)
        self.country_box.blockSignals(False)

    @staticmethod
    def _label(p: Place) -> str:
        marks = ("⭐" if p.is_favorite else "") + ("🏛" if p.is_famous else "")
        location = ", ".join(x for x in (p.city, p.country) if x)
        return f"{marks + ' ' if marks else ''}{p.name}" + (f" — {location}" if location else "")

    def _current(self) -> Place | None:
        row = self.list.currentRow()
        return self.places[row] if 0 <= row < len(self.places) else None

    # ── карта ───────────────────────────────────

    def _on_map_loaded(self, ok: bool):
        if not ok:
            print("карта: страница не загрузилась")
            return
        self._map_ready = True
        self._push_to_map()

    def _push_to_map(self):
        if not self._map_ready:
            return
        payload = json.dumps([p.to_json() for p in self.places], ensure_ascii=False)
        self.map_view.page().runJavaScript(f"setPlaces({payload})")

    def _select_by_id(self, place_id: int):
        for row, p in enumerate(self.places):
            if p.id == place_id:
                self.list.setCurrentRow(row)
                return

    def _on_select(self, row: int):
        p = self._current()
        if p is None:
            self.description.clear()
            return
        self.description.setHtml(
            f"<b>{p.name}</b> ({p.category})<br>"
            f"{', '.join(x for x in (p.city, p.country) if x)}<br><br>"
            f"{p.description or '<i>Нет описания</i>'}"
        )
        if (p.latitude or p.longitude) and self._map_ready:
            self.map_view.page().runJavaScript(
                f"focusPlace({p.latitude}, {p.longitude})")

    # ── действия ────────────────────────────────

    def _add(self):
        dialog = PlaceDialog(self.api, parent=self)
        if dialog.exec():
            try:
                self.api.create(dialog.get_place())
            except ApiError as e:
                QMessageBox.warning(self, "TravelGuide", str(e))
                return
            self._reload_countries()
            self.refresh()

    def _edit(self):
        p = self._current()
        if p is None:
            return
        dialog = PlaceDialog(self.api, place=p, parent=self)
        if dialog.exec():
            try:
                self.api.update(dialog.get_place())
            except NotFoundError:
                QMessageBox.warning(self, "TravelGuide",
                                    "Место уже удалено. Обновляю список.")
            except ApiError as e:
                QMessageBox.warning(self, "TravelGuide", str(e))
                return
            self._reload_countries()
            self.refresh()

    def _delete(self):
        p = self._current()
        if p is None:
            return
        answer = QMessageBox.question(self, "Удаление", f"Удалить «{p.name}»?")
        if answer != QMessageBox.StandardButton.Yes:
            return
        try:
            self.api.delete(p.id)
        except NotFoundError:
            pass  # уже удалено — результат тот же
        except ApiError as e:
            QMessageBox.warning(self, "TravelGuide", str(e))
            return
        self._reload_countries()
        self.refresh()

    def _toggle_favorite(self):
        p = self._current()
        if p is None:
            return
        try:
            # Бэкенд вернул новое состояние — обновляем строку локально,
            # без перезапроса всего списка.
            p.is_favorite = self.api.toggle_favorite(p.id)
        except ApiError as e:
            QMessageBox.warning(self, "TravelGuide", str(e))
            return
        self.list.currentItem().setText(self._label(p))
        self._push_to_map()

    def _toggle_famous(self):
        p = self._current()
        if p is None:
            return
        try:
            p.is_famous = self.api.toggle_famous(p.id)
        except ApiError as e:
            QMessageBox.warning(self, "TravelGuide", str(e))
            return
        self.list.currentItem().setText(self._label(p))
        self._push_to_map()