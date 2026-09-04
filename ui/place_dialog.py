"""Диалог добавления/редактирования места с подсказками Nominatim."""

from PyQt6.QtCore import QThread, QTimer, pyqtSignal
from PyQt6.QtWidgets import (
    QCheckBox, QComboBox, QDialog, QDialogButtonBox, QDoubleSpinBox,
    QFormLayout, QLabel, QLineEdit, QListWidget, QMessageBox, QTextEdit,
    QVBoxLayout,
)

from api_client import ApiClient, ApiError, Place
from constants import CATEGORIES


class GeocodeWorker(QThread):
    """Запрос к /api/geocode в фоне: Nominatim через наш бэкенд может
    отвечать секунду-другую (лимит 1 req/s), UI блокировать нельзя."""
    done = pyqtSignal(list)
    failed = pyqtSignal(str)

    def __init__(self, api: ApiClient, query: str):
        super().__init__()
        self.api = api
        self.query = query

    def run(self):
        try:
            self.done.emit(self.api.geocode(self.query))
        except ApiError as e:
            self.failed.emit(str(e))


class PlaceDialog(QDialog):
    def __init__(self, api: ApiClient, place: Place | None = None, parent=None):
        super().__init__(parent)
        self.api = api
        self.place = place or Place()
        self._worker: GeocodeWorker | None = None
        self._suggestions: list[dict] = []

        self.setWindowTitle("Редактирование места" if place else "Новое место")
        self.setMinimumWidth(480)
        self._build_ui()
        self._fill_from_place()

    def _build_ui(self):
        self.name = QLineEdit()
        self.hint = QLabel("Начните вводить название — появятся подсказки")
        self.hint.setStyleSheet("color: gray")

        # Дебаунс 400 мс: запрос уходит только после паузы во вводе.
        self._debounce = QTimer(self, singleShot=True, interval=400)
        self._debounce.timeout.connect(self._request_suggestions)
        self.name.textEdited.connect(self._debounce.start)

        self.suggestions = QListWidget()
        self.suggestions.setMaximumHeight(120)
        self.suggestions.hide()
        self.suggestions.itemClicked.connect(self._apply_suggestion)

        self.category = QComboBox()
        self.category.addItems(CATEGORIES)

        self.city = QLineEdit()

        # Редактируемый комбобокс стран: пользователь видит существующие
        # написания и выбирает их вместо того, чтобы печатать вариацию.
        self.country = QComboBox()
        self.country.setEditable(True)
        try:
            self.country.addItems(self.api.countries())
        except ApiError:
            pass  # без списка тоже работает — просто как текстовое поле

        self.description = QTextEdit()
        self.description.setMaximumHeight(100)

        self.lat = QDoubleSpinBox(minimum=-90, maximum=90, decimals=6)
        self.lon = QDoubleSpinBox(minimum=-180, maximum=180, decimals=6)

        self.favorite = QCheckBox("⭐ Избранное")
        self.famous = QCheckBox("🏛 Знаменитое")

        form = QFormLayout()
        form.addRow("Название:", self.name)
        form.addRow("", self.hint)
        form.addRow("", self.suggestions)
        form.addRow("Категория:", self.category)
        form.addRow("Город:", self.city)
        form.addRow("Страна:", self.country)
        form.addRow("Описание:", self.description)
        form.addRow("Широта:", self.lat)
        form.addRow("Долгота:", self.lon)
        form.addRow("", self.favorite)
        form.addRow("", self.famous)

        buttons = QDialogButtonBox(
            QDialogButtonBox.StandardButton.Save
            | QDialogButtonBox.StandardButton.Cancel)
        buttons.accepted.connect(self._validate_and_accept)
        buttons.rejected.connect(self.reject)

        layout = QVBoxLayout(self)
        layout.addLayout(form)
        layout.addWidget(buttons)

    def _fill_from_place(self):
        p = self.place
        self.name.setText(p.name)
        idx = self.category.findText(p.category)
        self.category.setCurrentIndex(idx if idx >= 0 else self.category.count() - 1)
        self.city.setText(p.city)
        self.country.setEditText(p.country)
        self.description.setPlainText(p.description)
        self.lat.setValue(p.latitude)
        self.lon.setValue(p.longitude)
        self.favorite.setChecked(p.is_favorite)
        self.famous.setChecked(p.is_famous)

    # ── подсказки ───────────────────────────────

    def _request_suggestions(self):
        query = self.name.text().strip()
        if len(query) < 3:  # то же ограничение, что и на бэкенде
            self.suggestions.hide()
            return
        if self._worker is not None and self._worker.isRunning():
            return  # предыдущий запрос ещё в пути; новый уйдёт по следующему дебаунсу
        self.hint.setText("Ищу…")
        self._worker = GeocodeWorker(self.api, query)
        self._worker.done.connect(self._show_suggestions)
        self._worker.failed.connect(self._suggestions_failed)
        self._worker.start()

    def _show_suggestions(self, items: list):
        self._suggestions = items
        self.suggestions.clear()
        for s in items:
            self.suggestions.addItem(s["display_name"])
        self.suggestions.setVisible(bool(items))
        self.hint.setText("Выберите подсказку или заполните поля вручную"
                          if items else "Ничего не найдено — заполните вручную")

    def _suggestions_failed(self, msg: str):
        self.hint.setText(f"Подсказки недоступны ({msg}) — заполните вручную")
        self.suggestions.hide()

    def _apply_suggestion(self, item):
        s = self._suggestions[self.suggestions.row(item)]
        # Названия приходят уже на русском (accept-language=ru,en
        # на стороне Go-клиента Nominatim) — дубли стран не плодятся.
        if s.get("name"):
            self.name.setText(s["name"])
        self.city.setText(s.get("city", ""))
        self.country.setEditText(s.get("country", ""))
        self.lat.setValue(s.get("latitude", 0.0))
        self.lon.setValue(s.get("longitude", 0.0))
        self.suggestions.hide()

    # ── результат ───────────────────────────────

    def _validate_and_accept(self):
        if not self.name.text().strip():
            QMessageBox.warning(self, "TravelGuide", "Название обязательно.")
            return
        self.accept()

    def get_place(self) -> Place:
        p = self.place
        p.name = self.name.text().strip()
        p.category = self.category.currentText()
        p.city = self.city.text().strip()
        p.country = self.country.currentText().strip()
        p.description = self.description.toPlainText().strip()
        p.latitude = self.lat.value()
        p.longitude = self.lon.value()
        p.is_favorite = self.favorite.isChecked()
        p.is_famous = self.famous.isChecked()
        return p