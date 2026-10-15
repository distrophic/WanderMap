# WanderMap

Локальный справочник мест: список, карта, поиск через Nominatim и свои фотографии. Данные лежат на этом компьютере, отдельный сервер не нужен.

Окно — Python и PyQt6. Логика и база — программа на Go. Страница в браузере открывается по адресу http://127.0.0.1:8080/, пока окно запущено.

## Переезд на новый ноутбук

1. Установите [Git](https://git-scm.com/downloads), [Go](https://go.dev/dl/) и Python 3.12 с официального сайта. На Windows нужен лаунчер `py`. После установки закройте терминал и откройте новый: иначе `go` может не находиться.
2. Склонируйте репозиторий и перейдите в него:

```powershell
git clone https://github.com/distrophic/WanderMap.git
cd WanderMap
```

3. Скопируйте со старого компьютера папку с базой и фотографиями:

`C:\Users\<имя>\AppData\Roaming\travelguide`

Положите её в то же место на новом компьютере. Внутри лежат `places.db` и каталог `photos`. Без этой папки программа откроется с пустым списком.

4. Установите библиотеки окна и соберите программу:

```powershell
py -m pip install -r ui/requirements.txt
cd backend
go build -o travelguide.exe ./cmd/travelguide
cd ..
```

На Linux и macOS файл называется `travelguide`, без `.exe`.

5. Из корня репозитория запустите:

```powershell
py ui/main.py
```

Команда сама поднимает Go-программу. Страница: http://127.0.0.1:8080/

## Если команда не находится

- `python` на Windows часто отсутствует в `PATH`. Используйте `py`.
- `go` не виден в терминале, открытом до установки. Откройте новое окно или выполните:

```powershell
$env:Path = [Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User')
go version
```

Проверка: `go version` и `py -m pip --version` печатают версии без ошибки.

## Где что лежит

| Что | Где |
|---|---|
| База мест | `%AppData%\travelguide\places.db` |
| Фотографии | `%AppData%\travelguide\photos` |
| Окно | `ui/main.py` |
| Страница браузера | `ui/web/index.html` |
| API и база | `backend/` |

На Linux база обычно в `~/.config/travelguide/`.
