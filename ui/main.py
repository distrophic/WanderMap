"""Точка входа. Поднимает Go-бэкенд, если тот ещё не запущен, и открывает окно."""

import subprocess
import sys
import time
from pathlib import Path

import os 

import requests
from PyQt6.QtGui import QFont, QFontDatabase
from PyQt6.QtWidgets import QApplication, QMessageBox

from api_client import BASE_URL, ApiClient
from main_window import MainWindow

os.environ["QTWEBENGINE_CHROMIUM_FLAGS"] = "--disable-gpu"

ASSETS = Path(__file__).parent.parent / "assets"

def ensure_backend() -> subprocess.Popen | None:
    """Возвращает процесс бэкенда, если пришлось запускать его самим."""
    try:
        requests.get(BASE_URL + "/api/countries", timeout=1)
        return None  # уже работает (например, запущен через systemd)
    except requests.RequestException:
        pass

    # Ищем бинарник рядом с проектом; на этапе Linux-упаковки
    # путь станет /usr/bin/travelguide или systemd возьмёт запуск на себя.
    candidates = [
        Path(__file__).parent.parent / "backend" / "travelguide",
        Path("travelguide"),
    ]
    exe = next((p for p in candidates if p.exists()), None)
    if exe is None:
        raise RuntimeError(
            "Бэкенд не запущен и бинарник не найден.\n"
            "Соберите его: cd backend && go build ./cmd/travelguide"
        )

    proc = subprocess.Popen([str(exe)])
    for _ in range(25):  # ждём до 5 секунд
        time.sleep(0.2)
        try:
            requests.get(BASE_URL + "/api/countries", timeout=1)
            return proc
        except requests.RequestException:
            continue
    proc.terminate()
    raise RuntimeError("Бэкенд запустился, но не отвечает.")

def load_fonts(app: QApplication):
    """Подключает Inter; если файлов нет — остаёмся на системном шрифте."""
    loaded = False
    for f in ("Inter-Regular.ttf", "Inter-Medium.ttf", "Inter-Bold.ttf"):
        path = ASSETS / "fonts" / f
        if path.exists() and QFontDatabase.addApplicationFont(str(path)) != -1:
            loaded = True
    if loaded:
        app.setFont(QFont("Inter", 10))


def load_styles(app: QApplication):
    qss = ASSETS / "style.qss"
    print("QSS путь:", qss)
    print("QSS существует:", qss.exists())
    if qss.exists():
        text = qss.read_text(encoding="utf-8")
        print("QSS длина:", len(text), "символов")
        app.setStyleSheet(text)
    else:
        print("QSS НЕ НАЙДЕН — стили не применены")

def main() -> int:
    print(">>> main() стартовал")
    app = QApplication(sys.argv)
    app.setApplicationName("TravelGuide")

    load_fonts(app)
    load_styles(app)
    print(">>> стили загружены")

    try:
        backend = ensure_backend()
    except RuntimeError as e:
        QMessageBox.critical(None, "TravelGuide", str(e))
        return 1

    window = MainWindow(ApiClient())
    window.show()
    code = app.exec()

    if backend is not None:
        backend.terminate()  # Go-сторона ловит SIGTERM и делает graceful shutdown
        backend.wait(timeout=5)
    return code

if __name__ == "__main__":
    sys.exit(main())