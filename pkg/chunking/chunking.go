package chunking

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// FileChunk представляет один кусок файла
type FileChunk struct {
	ID       string `json:"id"`       // уникальный идентификатор куска
	Index    int    `json:"index"`    // номер куска (0-5)
	FileID   string `json:"file_id"`  // идентификатор исходного файла
	Size     int64  `json:"size"`     // размер куска в байтах
	Checksum string `json:"checksum"` // контрольная сумма куска
	Data     []byte `json:"data"`     // данные куска
}

// FileMetadata содержит метаданные файла
type FileMetadata struct {
	ID           string      `json:"id"`            // уникальный идентификатор файла
	OriginalName string      `json:"original_name"` // оригинальное имя файла
	Size         int64       `json:"size"`          // размер файла в байтах
	Checksum     string      `json:"checksum"`      // контрольная сумма файла
	ChunkCount   int         `json:"chunk_count"`   // количество кусков
	Chunks       []FileChunk `json:"chunks"`        // информация о кусках
	ContentType  string      `json:"content_type"`  // MIME тип файла
}

// ChunkFile разделяет файл на заданное количество частей
func ChunkFile(filePath string, chunkCount int, fileID string) (*FileMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл: %w", err)
	}
	defer file.Close()

	// Получаем информацию о файле
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("не удалось получить информацию о файле: %w", err)
	}

	fileSize := fileInfo.Size()
	chunkSize := fileSize / int64(chunkCount)
	remainder := fileSize % int64(chunkCount)

	// Вычисляем контрольную сумму всего файла
	file.Seek(0, 0)
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, fmt.Errorf("не удалось вычислить контрольную сумму файла: %w", err)
	}
	fileChecksum := fmt.Sprintf("%x", hasher.Sum(nil))

	metadata := &FileMetadata{
		ID:           fileID,
		OriginalName: fileInfo.Name(),
		Size:         fileSize,
		Checksum:     fileChecksum,
		ChunkCount:   chunkCount,
		Chunks:       make([]FileChunk, chunkCount),
	}

	// Разделяем файл на куски
	file.Seek(0, 0)
	for i := 0; i < chunkCount; i++ {
		currentChunkSize := chunkSize
		// Последний кусок получает остаток
		if i == chunkCount-1 {
			currentChunkSize += remainder
		}

		chunkData := make([]byte, currentChunkSize)
		_, err := io.ReadFull(file, chunkData)
		if err != nil {
			return nil, fmt.Errorf("не удалось прочитать кусок %d: %w", i, err)
		}

		// Вычисляем контрольную сумму куска
		chunkHasher := sha256.New()
		chunkHasher.Write(chunkData)
		chunkChecksum := fmt.Sprintf("%x", chunkHasher.Sum(nil))

		chunk := FileChunk{
			ID:       fmt.Sprintf("%s_chunk_%d", fileID, i),
			Index:    i,
			FileID:   fileID,
			Size:     currentChunkSize,
			Checksum: chunkChecksum,
			Data:     chunkData,
		}

		metadata.Chunks[i] = chunk
	}

	return metadata, nil
}

// ReconstructFile собирает файл из кусков
func ReconstructFile(chunks []FileChunk, outputPath string) error {
	if len(chunks) == 0 {
		return fmt.Errorf("нет кусков для сборки файла")
	}

	// Сортируем куски по индексу
	for i := 0; i < len(chunks); i++ {
		for j := i + 1; j < len(chunks); j++ {
			if chunks[i].Index > chunks[j].Index {
				chunks[i], chunks[j] = chunks[j], chunks[i]
			}
		}
	}

	// Проверяем, что все куски на месте
	for i, chunk := range chunks {
		if chunk.Index != i {
			return fmt.Errorf("отсутствует кусок с индексом %d", i)
		}
	}

	// Создаем выходной файл
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("не удалось создать выходной файл: %w", err)
	}
	defer outputFile.Close()

	// Записываем куски в файл
	for _, chunk := range chunks {
		if _, err := outputFile.Write(chunk.Data); err != nil {
			return fmt.Errorf("не удалось записать кусок %d: %w", chunk.Index, err)
		}
	}

	return nil
}

// ValidateChunk проверяет целостность куска
func ValidateChunk(chunk *FileChunk) error {
	if chunk.Data == nil {
		return fmt.Errorf("данные куска отсутствуют")
	}

	if int64(len(chunk.Data)) != chunk.Size {
		return fmt.Errorf("размер данных не соответствует заявленному размеру")
	}

	// Проверяем контрольную сумму
	hasher := sha256.New()
	hasher.Write(chunk.Data)
	checksum := fmt.Sprintf("%x", hasher.Sum(nil))

	if checksum != chunk.Checksum {
		return fmt.Errorf("контрольная сумма куска не совпадает")
	}

	return nil
}

// ValidateFileMetadata проверяет целостность метаданных файла
func ValidateFileMetadata(metadata *FileMetadata) error {
	if len(metadata.Chunks) != metadata.ChunkCount {
		return fmt.Errorf("количество кусков не соответствует заявленному")
	}

	var totalSize int64
	for i, chunk := range metadata.Chunks {
		if chunk.Index != i {
			return fmt.Errorf("неправильный индекс куска: ожидался %d, получен %d", i, chunk.Index)
		}
		if chunk.FileID != metadata.ID {
			return fmt.Errorf("идентификатор файла в куске %d не соответствует метаданным", i)
		}
		totalSize += chunk.Size
	}

	if totalSize != metadata.Size {
		return fmt.Errorf("общий размер кусков не соответствует размеру файла")
	}

	return nil
}
