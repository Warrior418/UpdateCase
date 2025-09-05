package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"TestCase/internal/config"
	"TestCase/pkg/chunking"
	"TestCase/pkg/storage"
)

// MemoryStorageServer представляет сервер хранения с использованием памяти
type MemoryStorageServer struct {
	config        *config.Config
	memoryStorage *storage.MemoryStorage
	serverID      string
}

// NewMemoryStorageServer создает новый сервер хранения в памяти
func NewMemoryStorageServer(cfg *config.Config, serverID string) *MemoryStorageServer {
	return &MemoryStorageServer{
		config:        cfg,
		memoryStorage: storage.NewMemoryStorage(),
		serverID:      serverID,
	}
}

// setupMemoryRoutes настраивает маршруты для сервера хранения в памяти
func (s *MemoryStorageServer) setupMemoryRoutes() *gin.Engine {
	router := gin.Default()

	// Middleware для логирования
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Проверка здоровья сервиса
	router.GET("/health", s.healthCheck)

	// API для работы с кусками файлов
	v1 := router.Group("/api/v1")
	{
		v1.POST("/chunks", s.storeChunk)
		v1.GET("/chunks/:id", s.getChunk)
		v1.DELETE("/chunks/:id", s.deleteChunk)
		v1.GET("/chunks", s.listChunks)
		v1.GET("/info", s.getStorageInfo)
		v1.GET("/memory", s.getMemoryUsage)
		v1.POST("/compact", s.compactStorage)
	}

	return router
}

// healthCheck проверяет состояние сервиса хранения
func (s *MemoryStorageServer) healthCheck(c *gin.Context) {
	// Проверяем доступность хранилища в памяти
	_, err := s.memoryStorage.GetStorageInfo()
	status := "healthy"
	if err != nil {
		status = "unhealthy"
		log.Printf("Проблема с хранилищем в памяти: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    status,
		"server_id": s.serverID,
		"timestamp": time.Now().Unix(),
	})
}

// storeChunk сохраняет кусок файла в памяти
func (s *MemoryStorageServer) storeChunk(c *gin.Context) {
	var chunk chunking.FileChunk
	
	if err := c.ShouldBindJSON(&chunk); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных куска"})
		return
	}

	// Проверяем целостность куска
	if err := chunking.ValidateChunk(&chunk); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Кусок поврежден: %v", err)})
		return
	}

	// Сохраняем кусок в памяти
	if err := s.memoryStorage.StoreChunk(&chunk); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось сохранить кусок: %v", err)})
		return
	}

	log.Printf("Кусок %s сохранен в памяти на сервере %s", chunk.ID, s.serverID)
	c.JSON(http.StatusOK, gin.H{
		"message":   "Кусок успешно сохранен",
		"chunk_id":  chunk.ID,
		"server_id": s.serverID,
	})
}

// getChunk получает кусок файла из памяти
func (s *MemoryStorageServer) getChunk(c *gin.Context) {
	chunkID := c.Param("id")

	chunk, err := s.memoryStorage.GetChunk(chunkID)
	if err != nil {
		if err.Error() == "кусок не найден" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Кусок не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось получить кусок: %v", err)})
		}
		return
	}

	c.JSON(http.StatusOK, chunk)
}

// deleteChunk удаляет кусок файла из памяти
func (s *MemoryStorageServer) deleteChunk(c *gin.Context) {
	chunkID := c.Param("id")

	if err := s.memoryStorage.DeleteChunk(chunkID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось удалить кусок: %v", err)})
		return
	}

	log.Printf("Кусок %s удален из памяти на сервере %s", chunkID, s.serverID)
	c.JSON(http.StatusOK, gin.H{
		"message":   "Кусок успешно удален",
		"chunk_id":  chunkID,
		"server_id": s.serverID,
	})
}

// listChunks возвращает список всех кусков в памяти
func (s *MemoryStorageServer) listChunks(c *gin.Context) {
	chunks, err := s.memoryStorage.ListChunks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось получить список кусков: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"chunks":    chunks,
		"count":     len(chunks),
		"server_id": s.serverID,
	})
}

// getStorageInfo возвращает информацию о хранилище
func (s *MemoryStorageServer) getStorageInfo(c *gin.Context) {
	info, err := s.memoryStorage.GetStorageInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось получить информацию о хранилище: %v", err)})
		return
	}

	info["server_id"] = s.serverID
	c.JSON(http.StatusOK, info)
}

// getMemoryUsage возвращает информацию об использовании памяти
func (s *MemoryStorageServer) getMemoryUsage(c *gin.Context) {
	usage, err := s.memoryStorage.GetMemoryUsage()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось получить информацию о памяти: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"memory_usage_bytes": usage,
		"memory_usage_mb":    float64(usage) / (1024 * 1024),
		"server_id":          s.serverID,
	})
}

// compactStorage очищает память от неиспользуемых кусков
func (s *MemoryStorageServer) compactStorage(c *gin.Context) {
	compacted := s.memoryStorage.CompactStorage()
	
	c.JSON(http.StatusOK, gin.H{
		"message":        "Память очищена",
		"chunks_removed": compacted,
		"server_id":      s.serverID,
	})
}

func mainMemory() {
	// Получаем ID сервера из переменной окружения или используем значение по умолчанию
	serverID := os.Getenv("SERVER_ID")
	if serverID == "" {
		serverID = "1"
	}

	// Получаем порт сервера из переменной окружения
	port := os.Getenv("STORAGE_PORT")
	if port == "" {
		port = "8081"
	}

	// Загружаем конфигурацию
	cfg := config.NewConfig()
	cfg.StoragePort = port

	// Создаем сервер хранения в памяти
	server := NewMemoryStorageServer(cfg, serverID)

	// Настраиваем маршруты
	router := server.setupMemoryRoutes()

	// Запускаем сервер
	address := fmt.Sprintf(":%s", port)
	log.Printf("Запуск сервера хранения в памяти %s на порту %s", serverID, port)
	
	if err := router.Run(address); err != nil {
		log.Fatalf("Не удалось запустить сервер: %v", err)
	}
}

// main запускает сервер хранения в памяти
func main() {
	mainMemory()
}
