package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config содержит конфигурацию приложения
type Config struct {
	// Настройки API сервера
	APIPort   string
	APIHost   string
	
	// Настройки серверов хранения
	StorageServers []string
	StoragePort    string
	
	// Настройки файлов
	MaxFileSize   int64  // в байтах
	ChunkCount    int    // количество частей для разделения файла
	UploadDir     string // директория для временных файлов
	StorageDir    string // директория для хранения частей файлов
}

// NewConfig создает новую конфигурацию с значениями по умолчанию
func NewConfig() *Config {
	return &Config{
		APIPort:        getEnv("API_PORT", "8080"),
		APIHost:        getEnv("API_HOST", "0.0.0.0"),
		StoragePort:    getEnv("STORAGE_PORT", "8081"),
		MaxFileSize:    getEnvInt64("MAX_FILE_SIZE", 10*1024*1024*1024), // 10 GiB
		ChunkCount:     getEnvInt("CHUNK_COUNT", 6),
		UploadDir:      getEnv("UPLOAD_DIR", "./uploads"),
		StorageDir:     getEnv("STORAGE_DIR", "./storage"),
		StorageServers: getEnvSlice("STORAGE_SERVERS", []string{"localhost:8081", "localhost:8082", "localhost:8083", "localhost:8084", "localhost:8085", "localhost:8086"}),
	}
}

// getEnv возвращает значение переменной окружения или значение по умолчанию
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt возвращает значение переменной окружения как int или значение по умолчанию
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvInt64 возвращает значение переменной окружения как int64 или значение по умолчанию
func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvSlice возвращает значение переменной окружения как слайс строк или значение по умолчанию
func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

// GetAPIAddress возвращает полный адрес API сервера
func (c *Config) GetAPIAddress() string {
	return fmt.Sprintf("%s:%s", c.APIHost, c.APIPort)
}

// GetStorageAddress возвращает адрес сервера хранения по индексу
func (c *Config) GetStorageAddress(index int) string {
	if index < 0 || index >= len(c.StorageServers) {
		return ""
	}
	return c.StorageServers[index]
}

// GetStorageCount возвращает количество серверов хранения
func (c *Config) GetStorageCount() int {
	return len(c.StorageServers)
}
