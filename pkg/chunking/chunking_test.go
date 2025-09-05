package chunking

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkFile(t *testing.T) {
	// Создаем временный файл для тестирования
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testData := []byte("Hello, World! This is a test file for chunking functionality.")

	err := os.WriteFile(testFile, testData, 0644)
	require.NoError(t, err)

	// Тестируем разделение файла на 6 частей
	fileID := "test-file-id"
	metadata, err := ChunkFile(testFile, 6, fileID)

	require.NoError(t, err)
	assert.Equal(t, fileID, metadata.ID)
	assert.Equal(t, "test.txt", metadata.OriginalName)
	assert.Equal(t, int64(len(testData)), metadata.Size)
	assert.Equal(t, 6, metadata.ChunkCount)
	assert.Len(t, metadata.Chunks, 6)

	// Проверяем, что общий размер кусков равен размеру файла
	var totalSize int64
	for _, chunk := range metadata.Chunks {
		totalSize += chunk.Size
		assert.Equal(t, fileID, chunk.FileID)
		assert.NotEmpty(t, chunk.ID)
		assert.NotEmpty(t, chunk.Checksum)
		assert.NotNil(t, chunk.Data)
	}
	assert.Equal(t, metadata.Size, totalSize)

	// Проверяем валидацию метаданных
	err = ValidateFileMetadata(metadata)
	assert.NoError(t, err)

	// Проверяем валидацию каждого куска
	for _, chunk := range metadata.Chunks {
		err = ValidateChunk(&chunk)
		assert.NoError(t, err)
	}
}

func TestReconstructFile(t *testing.T) {
	// Создаем временный файл для тестирования
	tempDir := t.TempDir()
	originalFile := filepath.Join(tempDir, "original.txt")
	reconstructedFile := filepath.Join(tempDir, "reconstructed.txt")
	testData := []byte("This is a test file for reconstruction functionality. It contains multiple sentences to test chunking and reconstruction properly.")

	err := os.WriteFile(originalFile, testData, 0644)
	require.NoError(t, err)

	// Разделяем файл на куски
	fileID := "test-reconstruction"
	metadata, err := ChunkFile(originalFile, 6, fileID)
	require.NoError(t, err)

	// Восстанавливаем файл из кусков
	err = ReconstructFile(metadata.Chunks, reconstructedFile)
	require.NoError(t, err)

	// Проверяем, что восстановленный файл идентичен оригиналу
	reconstructedData, err := os.ReadFile(reconstructedFile)
	require.NoError(t, err)
	assert.Equal(t, testData, reconstructedData)
}

func TestValidateChunk(t *testing.T) {
	// Создаем валидный кусок
	validChunk := &FileChunk{
		ID:       "test-chunk",
		Index:    0,
		FileID:   "test-file",
		Size:     5,
		Checksum: "2cf24dba4f21d4288094e9b259d1c8d374c8b0c9b6c9e8a5f6e8f3c4e6d8a2b1", // SHA256 of "hello"
		Data:     []byte("hello"),
	}

	// Пересчитываем контрольную сумму для корректного теста
	validChunk.Checksum = "2cf24dba4f21d4288094e9b259d1c8d374c8b0c9b6c9e8a5f6e8f3c4e6d8a2b1"
	// Реальная контрольная сумма для "hello"
	testData := []byte("hello")
	metadata, _ := ChunkFile(createTempFile(t, testData), 1, "test")
	validChunk.Checksum = metadata.Chunks[0].Checksum
	validChunk.Data = testData

	err := ValidateChunk(validChunk)
	assert.NoError(t, err)

	// Тест с отсутствующими данными
	invalidChunk := *validChunk
	invalidChunk.Data = nil
	err = ValidateChunk(&invalidChunk)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "данные куска отсутствуют")

	// Тест с неправильным размером
	invalidChunk = *validChunk
	invalidChunk.Size = 10
	err = ValidateChunk(&invalidChunk)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "размер данных не соответствует")

	// Тест с неправильной контрольной суммой
	invalidChunk = *validChunk
	invalidChunk.Checksum = "invalid-checksum"
	err = ValidateChunk(&invalidChunk)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "контрольная сумма куска не совпадает")
}

func TestValidateFileMetadata(t *testing.T) {
	tempFile := createTempFile(t, []byte("test data for validation"))
	metadata, err := ChunkFile(tempFile, 3, "test-file")
	require.NoError(t, err)

	// Валидные метаданные
	err = ValidateFileMetadata(metadata)
	assert.NoError(t, err)

	// Неправильное количество кусков
	invalidMetadata := *metadata
	invalidMetadata.ChunkCount = 5
	err = ValidateFileMetadata(&invalidMetadata)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "количество кусков не соответствует")

	// Неправильный индекс куска
	invalidMetadata = *metadata
	invalidMetadata.Chunks[1].Index = 5
	err = ValidateFileMetadata(&invalidMetadata)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "неправильный индекс куска")

	// Неправильный ID файла в куске
	invalidMetadata = *metadata
	invalidMetadata.Chunks[0].FileID = "wrong-file-id"
	err = ValidateFileMetadata(&invalidMetadata)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "идентификатор файла в куске")
}

func TestChunkFileWithDifferentSizes(t *testing.T) {
	testCases := []struct {
		name       string
		dataSize   int
		chunkCount int
	}{
		{"Small file", 10, 3},
		{"Medium file", 1000, 6},
		{"Large file", 10000, 6},
		{"Single byte", 1, 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Создаем тестовые данные
			testData := make([]byte, tc.dataSize)
			for i := range testData {
				testData[i] = byte(i % 256)
			}

			tempFile := createTempFile(t, testData)

			// Разделяем файл
			metadata, err := ChunkFile(tempFile, tc.chunkCount, "test-file")
			require.NoError(t, err)

			// Проверяем базовые свойства
			assert.Equal(t, tc.chunkCount, len(metadata.Chunks))
			assert.Equal(t, int64(tc.dataSize), metadata.Size)

			// Проверяем, что можем восстановить файл
			tempDir := t.TempDir()
			reconstructedFile := filepath.Join(tempDir, "reconstructed")
			err = ReconstructFile(metadata.Chunks, reconstructedFile)
			require.NoError(t, err)

			// Проверяем идентичность
			reconstructedData, err := os.ReadFile(reconstructedFile)
			require.NoError(t, err)
			assert.Equal(t, testData, reconstructedData)
		})
	}
}

// Вспомогательная функция для создания временного файла
func createTempFile(t *testing.T, data []byte) string {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "temp.txt")
	err := os.WriteFile(tempFile, data, 0644)
	require.NoError(t, err)
	return tempFile
}
