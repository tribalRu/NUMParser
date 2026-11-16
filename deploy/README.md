# Инструкция по развертыванию numParser

В этом каталоге собраны все необходимые компоненты для быстрого развертывания сервера `numParser`.

---

## Содержимое каталога

| Файл / Папка | Описание |
|---|---|
| `numParser` | Исполняемый файл для Linux x86-64 (статический бинарник) |
| `numParser.exe` | Исполняемый файл для Windows x86-64 |
| `config.yml` | Рабочий конфигурационный файл с комментариями на русском языке |
| `config.example.yml` | Шаблон чистого конфигурационного файла |
| `collections.json` | Файл шаблонов и кэша подборок |
| `numparser.service` | Systemd юнит для автозапуска и работы в фоне на Linux |
| `docker-compose.yml` | Конфигурация для запуска в Docker Compose |
| `Dockerfile` | Скрипт сборки контейнера |
| `start.sh` | Скрипт запуска для Linux |
| `start.bat` | Скрипт запуска для Windows |

---

## Вариант 1: Развертывание на Linux (Ubuntu / Debian / AlmaLinux)

Рекомендуемый способ для выделенного сервера или VPS.

### 1. Скопируйте файлы на сервер
Создайте папку на сервере (например, `/opt/numparser`) и скопируйте туда содержимое этого каталога:
```bash
sudo mkdir -p /opt/numparser
sudo cp numParser config.yml collections.json numparser.service /opt/numparser/
cd /opt/numparser
sudo chmod +x numParser
```

### 2. Настройте автозапуск через systemd
```bash
# Скопируйте файл службы в systemd
sudo cp numparser.service /etc/systemd/system/

# Перечитайте конфигурацию systemd и включите автозапуск
sudo systemctl daemon-reload
sudo systemctl enable --now numparser
```

### 3. Проверка статуса и логов
```bash
# Проверить статус службы
sudo systemctl status numparser

# Посмотреть логи работы в реальном времени
sudo journalctl -u numparser -f
```

---

## Вариант 2: Запуск в Docker Compose

Если вы используете Docker:

```bash
# Запуск контейнера в фоновом режиме
docker-compose up -d

# Просмотр логов
docker-compose logs -f
```

Данные базы и сгенерированные релизы будут сохраняться в примонтированных томах `./data/db` и `./data/releases`.

---

## Вариант 3: Запуск на Windows

1. Убедитесь, что файлы `numParser.exe`, `config.yml` и `collections.json` находятся в одной папке.
2. Запустите `start.bat` или дважды щелкните по `numParser.exe`.
3. Приложение откроет консоль и запустит веб-интерфейс на настроенном порту (по умолчанию `30000` или `38888`).

---

## Подключение приложения NUM на Android TV

1. Убедитесь, что сервер доступен по сети с телевизора/приставки.
2. Откройте веб-интерфейс в браузере: `http://<IP_СЕРВЕРА>:<ПОРТ>/` (например, `http://192.168.1.100:30000/`).
3. В настройках приложения **NUM** на Android TV укажите адрес вашего сервера `numParser`:
   ```text
   http://<IP_СЕРВЕРА>:<ПОРТ>
   ```
4. NUM автоматически подгрузит сформированные релизы (`movies_id.json`, `tv_id.json` и др.), упорядоченные в соответствии с настройками в `config.yml`.
