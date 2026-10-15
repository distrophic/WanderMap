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

def backend_candidates() -> list[Path]:
    """Имена бинарника: на Windows Go дописывает .exe, без него файл не находится."""
    root = Path(__file__).parent.parent
    names = ["travelguide.exe", "travelguide"] if sys.platform == "win32" else ["travelguide"]
    directories = [root / "backend", Path.cwd()]
    return [directory / name for directory in directories for name in names]


def ensure_backend() -> subprocess.Popen | None:
    """Возвращает процесс бэкенда, если пришлось запускать его самим."""
    try:
        requests.get(BASE_URL + "/api/countries", timeout=1)
        return None  # уже работает — этот процесс не наш, при выходе его не гасим
    except requests.RequestException:
        pass

    exe = next((path for path in backend_candidates() if path.is_file()), None)
    if exe is None:
        raise RuntimeError(
            "Бэкенд не запущен и бинарник не найден.\n"
            "Соберите его из каталога backend:\n"
            "go build -o travelguide.exe ./cmd/travelguide"
        )

    # Без консольного окна: пользователь видит только Qt.
    # CREATE_NEW_PROCESS_GROUP нужен, чтобы закрытие окна не задевало чужие процессы.
    flags = 0
    if sys.platform == "win32":
        flags = subprocess.CREATE_NEW_PROCESS_GROUP | subprocess.CREATE_NO_WINDOW

    proc = subprocess.Popen([str(exe)], creationflags=flags)
    for _ in range(25):  # ждём до 5 секунд
        time.sleep(0.2)
        if proc.poll() is not None:
            break
        try:
            requests.get(BASE_URL + "/api/countries", timeout=1)
            return proc
        except requests.RequestException:
            continue
    stop_backend(proc)
    raise RuntimeError("Бэкенд запустился, но не отвечает.")


def stop_backend(proc: subprocess.Popen) -> None:
    """Гасит только тот процесс, который подняли мы сами."""
    if proc.poll() is not None:
        return
    proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=5)

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
        stop_backend(backend)
    return code

if __name__ == "__main__":
    sys.exit(main())