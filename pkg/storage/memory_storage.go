package storage

import (
	"fmt"
	"sync"

	"TestCase/pkg/chunking"
)

// MemoryStorage представляет хранилище в памяти для оптимизации
type MemoryStorage struct {
	chunks map[string]*chunking.FileChunk
	mutex  sync.RWMutex
}

// NewMemoryStorage создает новое хранилище в памяти
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		chunks: make(map[string]*chunking.FileChunk),
	}
}

// StoreChunk сохраняет кусок файла в памяти
func (ms *MemoryStorage) StoreChunk(chunk *chunking.FileChunk) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	// Создаем копию куска для хранения
	chunkCopy := &chunking.FileChunk{
		ID:       chunk.ID,
		FileID:   chunk.FileID,
		Index:    chunk.Index,
		Data:     make([]byte, len(chunk.Data)),
		Checksum: chunk.Checksum,
		Size:     chunk.Size,
	}

	// Копируем данные
	copy(chunkCopy.Data, chunk.Data)

	ms.chunks[chunk.ID] = chunkCopy
	return nil
}

// GetChunk получает кусок файла из памяти
func (ms *MemoryStorage) GetChunk(chunkID string) (*chunking.FileChunk, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	chunk, exists := ms.chunks[chunkID]
	if !exists {
		return nil, fmt.Errorf("кусок не найден")
	}

	// Создаем копию для возврата
	chunkCopy := &chunking.FileChunk{
		ID:       chunk.ID,
		FileID:   chunk.FileID,
		Index:    chunk.Index,
		Data:     make([]byte, len(chunk.Data)),
		Checksum: chunk.Checksum,
		Size:     chunk.Size,
	}

	copy(chunkCopy.Data, chunk.Data)

	return chunkCopy, nil
}

// DeleteChunk удаляет кусок файла из памяти
func (ms *MemoryStorage) DeleteChunk(chunkID string) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	if _, exists := ms.chunks[chunkID]; !exists {
		return fmt.Errorf("кусок не найден")
	}

	delete(ms.chunks, chunkID)
	return nil
}

// ListChunks возвращает список всех кусков в памяти
func (ms *MemoryStorage) ListChunks() ([]string, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	chunks := make([]string, 0, len(ms.chunks))
	for chunkID := range ms.chunks {
		chunks = append(chunks, chunkID)
	}

	return chunks, nil
}

// GetStorageInfo возвращает информацию о хранилище
func (ms *MemoryStorage) GetStorageInfo() (map[string]interface{}, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	var totalSize int64
	for _, chunk := range ms.chunks {
		totalSize += int64(len(chunk.Data))
	}

	info := map[string]interface{}{
		"chunk_count":  len(ms.chunks),
		"total_size":   totalSize,
		"storage_type": "memory",
	}

	return info, nil
}

// GetMemoryUsage возвращает использование памяти
func (ms *MemoryStorage) GetMemoryUsage() (int64, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	var totalSize int64
	for _, chunk := range ms.chunks {
		totalSize += int64(len(chunk.Data))
	}

	return totalSize, nil
}

// ClearAll очищает все данные из памяти
func (ms *MemoryStorage) ClearAll() {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	ms.chunks = make(map[string]*chunking.FileChunk)
}

// CompactStorage очищает память от неиспользуемых кусков
func (ms *MemoryStorage) CompactStorage() int {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	// В реальном приложении здесь была бы логика очистки старых кусков
	// Пока просто возвращаем количество кусков
	return len(ms.chunks)
}
