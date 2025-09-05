package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"TestCase/internal/config"
	"TestCase/pkg/chunking"
	"TestCase/pkg/storage"
)

// StreamingAPIServer представляет оптимизированный API сервер с потоковой обработкой
type StreamingAPIServer struct {
	config         *config.Config
	storageClients []*storage.StorageClient
	fileMetadata   map[string]*chunking.FileMetadata
	metadataMutex  sync.RWMutex
}

// NewStreamingAPIServer создает новый потоковый API сервер
func NewStreamingAPIServer(cfg *config.Config) *StreamingAPIServer {
	server := &StreamingAPIServer{
		config:       cfg,
		fileMetadata: make(map[string]*chunking.FileMetadata),
	}

	// Создаем клиенты для серверов хранения
	for _, serverAddr := range cfg.StorageServers {
		client := storage.NewStorageClient(fmt.Sprintf("http://%s", serverAddr))
		server.storageClients = append(server.storageClients, client)
	}

	return server
}

// calculateChecksum вычисляет SHA256 контрольную сумму
func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// setupStreamingRoutes настраивает маршруты для потокового API
func (s *StreamingAPIServer) setupStreamingRoutes() *gin.Engine {
	router := gin.Default()

	// Middleware для логирования
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Проверка здоровья сервиса
	router.GET("/health", s.healthCheck)

	// API для работы с файлами
	v1 := router.Group("/api/v1")
	{
		v1.POST("/files", s.streamingUploadFile)
		v1.GET("/files/:id", s.streamingDownloadFile)
		v1.GET("/files/:id/info", s.getFileInfo)
		v1.DELETE("/files/:id", s.deleteFile)
		v1.GET("/files", s.listFiles)
	}

	return router
}

// healthCheck проверяет состояние сервиса
func (s *StreamingAPIServer) healthCheck(c *gin.Context) {
	// Проверяем доступность серверов хранения
	var healthyServers int
	for i, client := range s.storageClients {
		if err := client.HealthCheck(); err != nil {
			log.Printf("Сервер хранения %d недоступен: %v", i, err)
		} else {
			healthyServers++
		}
	}

	status := "healthy"
	if healthyServers < s.config.ChunkCount {
		status = "degraded"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":          status,
		"healthy_servers": healthyServers,
		"total_servers":   len(s.storageClients),
		"timestamp":       time.Now().Unix(),
	})
}

// streamingUploadFile обрабатывает загрузку файла с потоковой обработкой
func (s *StreamingAPIServer) streamingUploadFile(c *gin.Context) {
	// Получаем файл из формы
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось получить файл из запроса"})
		return
	}
	defer file.Close()

	// Проверяем размер файла
	if header.Size > s.config.MaxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Размер файла превышает максимально допустимый (%d байт)", s.config.MaxFileSize),
		})
		return
	}

	// Генерируем ID файла
	fileID := uuid.New().String()

	// Читаем файл в память по частям для chunking
	fileData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось прочитать файл"})
		return
	}

	// Разделяем файл на куски в памяти
	chunks, err := s.chunkFileInMemory(fileData, fileID, s.config.ChunkCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось разделить файл: %v", err)})
		return
	}

	// Создаем метаданные файла
	metadata := &chunking.FileMetadata{
		ID:           fileID,
		OriginalName: header.Filename,
		Size:         int64(len(fileData)),
		Checksum:     calculateChecksum(fileData),
		ContentType:  header.Header.Get("Content-Type"),
		ChunkCount:   len(chunks),
		Chunks:       chunks,
	}

	// Сохраняем куски на серверах хранения
	if err := s.distributeChunks(metadata); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось сохранить куски: %v", err)})
		return
	}

	// Сохраняем метаданные
	s.metadataMutex.Lock()
	s.fileMetadata[fileID] = metadata
	s.metadataMutex.Unlock()

	// Очищаем данные из памяти
	fileData = nil

	c.JSON(http.StatusOK, metadata)
}

// chunkFileInMemory разделяет файл на куски в памяти
func (s *StreamingAPIServer) chunkFileInMemory(data []byte, fileID string, chunkCount int) ([]chunking.FileChunk, error) {
	fileSize := len(data)
	chunkSize := fileSize / chunkCount

	chunks := make([]chunking.FileChunk, chunkCount)

	for i := 0; i < chunkCount; i++ {
		start := i * chunkSize
		end := start + chunkSize

		// Последний кусок получает все оставшиеся данные
		if i == chunkCount-1 {
			end = fileSize
		}

		chunkData := data[start:end]
		chunkID := fmt.Sprintf("%s_chunk_%d", fileID, i)

		chunks[i] = chunking.FileChunk{
			ID:       chunkID,
			FileID:   fileID,
			Index:    i,
			Data:     chunkData,
			Checksum: calculateChecksum(chunkData),
			Size:     int64(len(chunkData)),
		}
	}

	return chunks, nil
}

// distributeChunks распределяет куски файла по серверам хранения
func (s *StreamingAPIServer) distributeChunks(metadata *chunking.FileMetadata) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(metadata.Chunks))

	for i, chunk := range metadata.Chunks {
		wg.Add(1)
		go func(chunkIndex int, chunkData chunking.FileChunk) {
			defer wg.Done()

			// Выбираем сервер хранения (равномерное распределение)
			serverIndex := chunkIndex % len(s.storageClients)
			client := s.storageClients[serverIndex]

			// Пытаемся сохранить кусок
			if err := client.StoreChunk(&chunkData); err != nil {
				errChan <- fmt.Errorf("не удалось сохранить кусок %d на сервере %d: %w", chunkIndex, serverIndex, err)
				return
			}

			log.Printf("Кусок %d сохранен на сервере %d", chunkIndex, serverIndex)
		}(i, chunk)
	}

	wg.Wait()
	close(errChan)

	// Проверяем ошибки
	for err := range errChan {
		return err
	}

	return nil
}

// streamingDownloadFile обрабатывает скачивание файла с потоковой передачей
func (s *StreamingAPIServer) streamingDownloadFile(c *gin.Context) {
	fileID := c.Param("id")

	// Получаем метаданные файла
	s.metadataMutex.RLock()
	metadata, exists := s.fileMetadata[fileID]
	s.metadataMutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Файл не найден"})
		return
	}

	// Собираем куски файла
	chunks, err := s.collectChunks(metadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось собрать файл: %v", err)})
		return
	}

	// Собираем файл в памяти
	fileData, err := s.reconstructFileInMemory(chunks)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Не удалось собрать файл: %v", err)})
		return
	}

	// Отправляем файл клиенту потоково
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", metadata.OriginalName))
	c.Header("Content-Length", fmt.Sprintf("%d", len(fileData)))
	if metadata.ContentType != "" {
		c.Header("Content-Type", metadata.ContentType)
	}

	// Отправляем данные потоково
	reader := bytes.NewReader(fileData)
	c.DataFromReader(http.StatusOK, int64(len(fileData)), metadata.ContentType, reader, nil)
}

// reconstructFileInMemory собирает файл из кусков в памяти
func (s *StreamingAPIServer) reconstructFileInMemory(chunks []chunking.FileChunk) ([]byte, error) {
	var totalSize int
	for _, chunk := range chunks {
		totalSize += len(chunk.Data)
	}

	fileData := make([]byte, 0, totalSize)
	for _, chunk := range chunks {
		fileData = append(fileData, chunk.Data...)
	}

	return fileData, nil
}

// collectChunks собирает куски файла с серверов хранения
func (s *StreamingAPIServer) collectChunks(metadata *chunking.FileMetadata) ([]chunking.FileChunk, error) {
	chunks := make([]chunking.FileChunk, len(metadata.Chunks))
	var wg sync.WaitGroup
	errChan := make(chan error, len(metadata.Chunks))

	for i, chunkMeta := range metadata.Chunks {
		wg.Add(1)
		go func(chunkIndex int, chunkMetadata chunking.FileChunk) {
			defer wg.Done()

			// Выбираем сервер хранения
			serverIndex := chunkIndex % len(s.storageClients)
			client := s.storageClients[serverIndex]

			// Получаем кусок
			chunk, err := client.GetChunk(chunkMetadata.ID)
			if err != nil {
				errChan <- fmt.Errorf("не удалось получить кусок %d с сервера %d: %w", chunkIndex, serverIndex, err)
				return
			}

			chunks[chunkIndex] = *chunk
		}(i, chunkMeta)
	}

	wg.Wait()
	close(errChan)

	// Проверяем ошибки
	for err := range errChan {
		return nil, err
	}

	return chunks, nil
}

// getFileInfo возвращает информацию о файле
func (s *StreamingAPIServer) getFileInfo(c *gin.Context) {
	fileID := c.Param("id")

	s.metadataMutex.RLock()
	metadata, exists := s.fileMetadata[fileID]
	s.metadataMutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Файл не найден"})
		return
	}

	c.JSON(http.StatusOK, metadata)
}

// deleteFile удаляет файл
func (s *StreamingAPIServer) deleteFile(c *gin.Context) {
	fileID := c.Param("id")

	// Получаем метаданные файла
	s.metadataMutex.Lock()
	metadata, exists := s.fileMetadata[fileID]
	if !exists {
		s.metadataMutex.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "Файл не найден"})
		return
	}
	delete(s.fileMetadata, fileID)
	s.metadataMutex.Unlock()

	// Удаляем куски с серверов хранения
	var wg sync.WaitGroup
	for i, chunk := range metadata.Chunks {
		wg.Add(1)
		go func(chunkIndex int, chunkData chunking.FileChunk) {
			defer wg.Done()

			serverIndex := chunkIndex % len(s.storageClients)
			client := s.storageClients[serverIndex]

			if err := client.DeleteChunk(chunkData.ID); err != nil {
				log.Printf("Не удалось удалить кусок %d с сервера %d: %v", chunkIndex, serverIndex, err)
			}
		}(i, chunk)
	}

	wg.Wait()

	c.JSON(http.StatusOK, gin.H{"message": "Файл удален"})
}

// listFiles возвращает список всех файлов
func (s *StreamingAPIServer) listFiles(c *gin.Context) {
	s.metadataMutex.RLock()
	defer s.metadataMutex.RUnlock()

	files := make([]string, 0, len(s.fileMetadata))
	for fileID := range s.fileMetadata {
		files = append(files, fileID)
	}

	c.JSON(http.StatusOK, files)
}

func main() {
	// Загружаем конфигурацию
	cfg := config.NewConfig()

	// Создаем потоковый API сервер
	server := NewStreamingAPIServer(cfg)

	// Настраиваем маршруты
	router := server.setupStreamingRoutes()

	// Запускаем сервер
	address := cfg.GetAPIAddress()
	log.Printf("Запуск потокового API сервера на адресе %s", address)

	if err := router.Run(address); err != nil {
		log.Fatalf("Не удалось запустить сервер: %v", err)
	}
}
