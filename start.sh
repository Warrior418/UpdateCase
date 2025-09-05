#!/bin/bash

# Скрипт для запуска оптимизированной версии системы с использованием памяти
# Версия: 2.0

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Функции для цветного вывода
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${BLUE}[SYSTEM]${NC} $1"
}

# Функция для проверки доступности порта
check_port() {
    local port=$1
    local timeout=${2:-1}
    
    if timeout $timeout bash -c "</dev/tcp/localhost/$port" 2>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Функция для ожидания готовности сервиса
wait_for_service() {
    local url=$1
    local service_name=$2
    local max_attempts=${3:-15}
    local attempt=1
    
    print_status "Ожидание готовности $service_name..."
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$url" > /dev/null 2>&1; then
            print_status "$service_name готов"
            return 0
        fi
        
        echo -n "."
        sleep 1
        attempt=$((attempt + 1))
    done
    
    echo ""
    print_error "$service_name не отвечает после $max_attempts попыток"
    return 1
}

# Функция для очистки процессов при завершении
cleanup() {
    echo ""
    print_status "Остановка сервисов..."
    
    # Останавливаем все процессы
    pkill -f "api" 2>/dev/null || true
    pkill -f "storage" 2>/dev/null || true
    
    # Ждем завершения процессов
    sleep 2
    
    print_status "Все сервисы остановлены"
    exit 0
}

# Устанавливаем обработчик сигналов
trap cleanup SIGINT SIGTERM

print_header "Запуск оптимизированной системы распределенного хранения файлов"
print_header "Использование памяти вместо диска для экономии места"
echo ""

# Проверяем, что Go установлен
if ! command -v go &> /dev/null; then
    print_error "Go не установлен. Пожалуйста, установите Go 1.21 или выше"
    exit 1
fi

# Проверяем версию Go
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
REQUIRED_VERSION="1.21"

if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
    print_error "Требуется Go версии $REQUIRED_VERSION или выше. Текущая версия: $GO_VERSION"
    exit 1
fi

print_status "Go версии $GO_VERSION найден"

# Создаем необходимые директории
mkdir -p logs bin

# Очищаем старые логи
print_status "Очистка старых логов..."
rm -f logs/*.log

print_status "Сборка приложений..."

# Собираем API сервер
print_status "Сборка API сервера..."
if go build -o bin/api ./cmd/api/main.go; then
    print_status "API сервер собран успешно"
else
    print_error "Ошибка сборки API сервера"
    exit 1
fi

# Собираем storage серверы
print_status "Сборка storage серверов..."
if go build -o bin/storage ./cmd/storage/memory_server.go; then
    print_status "Storage серверы собраны успешно"
else
    print_error "Ошибка сборки storage серверов"
    exit 1
fi

print_status "Сборка завершена"

# Останавливаем старые процессы
print_status "Остановка старых процессов..."
pkill -f "api" 2>/dev/null || true
pkill -f "storage" 2>/dev/null || true
sleep 2

# Запускаем storage серверы в фоне
print_status "Запуск серверов хранения в памяти..."

STORAGE_SERVERS_STARTED=0

for i in {1..6}; do
    port=$((8080 + i))
    print_status "Запуск storage сервера $i на порту $port"
    
    # Проверяем, что порт свободен
    if check_port $port 1; then
        print_warning "Порт $port уже занят, пропускаем сервер $i"
        continue
    fi
    
    SERVER_ID=$i STORAGE_PORT=$port ./bin/storage > logs/storage_$i.log 2>&1 &
    STORAGE_SERVERS_STARTED=$((STORAGE_SERVERS_STARTED + 1))
    sleep 1
done

if [ $STORAGE_SERVERS_STARTED -eq 0 ]; then
    print_error "Не удалось запустить ни одного storage сервера"
    exit 1
fi

# Ждем запуска storage серверов
print_status "Ожидание запуска storage серверов..."
sleep 3

# Проверяем, что storage серверы запустились
print_status "Проверка storage серверов..."
HEALTHY_SERVERS=0

for i in {1..6}; do
    port=$((8080 + i))
    if wait_for_service "http://localhost:$port/health" "Storage сервер $i" 5; then
        HEALTHY_SERVERS=$((HEALTHY_SERVERS + 1))
    else
        print_warning "Storage сервер $i не отвечает"
    fi
done

if [ $HEALTHY_SERVERS -lt 3 ]; then
    print_error "Недостаточно storage серверов для работы системы (работает $HEALTHY_SERVERS из 6)"
    cleanup
    exit 1
fi

print_status "Запущено $HEALTHY_SERVERS storage серверов из 6"

# Запускаем API сервер
print_status "Запуск API сервера..."

# Проверяем, что порт 8080 свободен
if check_port 8080 1; then
    print_warning "Порт 8080 уже занят, останавливаем существующий процесс"
    pkill -f "api" 2>/dev/null || true
    sleep 2
fi

API_PORT=8080 STORAGE_SERVERS="localhost:8081,localhost:8082,localhost:8083,localhost:8084,localhost:8085,localhost:8086" ./bin/api > logs/api.log 2>&1 &

# Ждем запуска API сервера
if wait_for_service "http://localhost:8080/health" "API сервер" 10; then
    print_status "API сервер готов"
else
    print_error "API сервер не отвечает"
    cleanup
    exit 1
fi

echo ""
print_header "Система запущена успешно!"
echo ""

print_status "Доступные сервисы:"
echo "   • API сервер: http://localhost:8080"
echo "   • Storage серверы: http://localhost:8081-8086 (работает $HEALTHY_SERVERS)"
echo "   • Health check: http://localhost:8080/health"
echo ""

print_status "Полезные команды для тестирования:"
echo "   • Проверить здоровье системы:"
echo "     curl http://localhost:8080/health"
echo ""
echo "   • Загрузить файл:"
echo "     curl -X POST -F 'file=@test.txt' http://localhost:8080/api/v1/files"
echo ""
echo "   • Получить список файлов:"
echo "     curl http://localhost:8080/api/v1/files"
echo ""
echo "   • Скачать файл (замените FILE_ID на реальный ID):"
echo "     curl -o downloaded.txt http://localhost:8080/api/v1/files/FILE_ID"
echo ""
echo "   • Проверить использование памяти storage сервера:"
echo "     curl http://localhost:8081/api/v1/info"
echo ""

print_status "Мониторинг логов:"
echo "   • API сервер: tail -f logs/api.log"
echo "   • Storage серверы: tail -f logs/storage_*.log"
echo "   • Все логи: tail -f logs/*.log"
echo ""

print_status "Преимущества оптимизированной версии:"
echo "   • Нет временных файлов на диске"
echo "   • Экономия оперативной памяти"
echo "   • Потоковая обработка файлов"
echo "   • Автоматическая очистка памяти"
echo "   • Улучшенная обработка ошибок"
echo "   • Цветной вывод для удобства"
echo ""

print_status "Система готова к работе. Нажмите Ctrl+C для остановки всех сервисов"

# Показываем статус системы каждые 30 секунд
while true; do
    sleep 30
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        print_status "Система работает нормально ($(date))"
    else
        print_warning "API сервер не отвечает! ($(date))"
    fi
done
