package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"TestCase/pkg/chunking"
)

// APIClient представляет клиент для работы с API сервером
type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient создает новый клиент для API сервера
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // увеличенный таймаут для больших файлов
		},
	}
}

// UploadFile загружает файл на сервер
func (ac *APIClient) UploadFile(filePath string) (*chunking.FileMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл: %w", err)
	}
	defer file.Close()

	// Создаем multipart форму
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	// Добавляем файл в форму
	fileWriter, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("не удалось создать форму файла: %w", err)
	}

	if _, err := io.Copy(fileWriter, file); err != nil {
		return nil, fmt.Errorf("не удалось скопировать файл в форму: %w", err)
	}

	writer.Close()

	// Отправляем запрос
	url := fmt.Sprintf("%s/api/v1/files", ac.baseURL)
	req, err := http.NewRequest("POST", url, &buffer)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать запрос: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("не удалось отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("сервер вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	// Читаем ответ
	var metadata chunking.FileMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("не удалось десериализовать ответ: %w", err)
	}

	return &metadata, nil
}

// DownloadFile скачивает файл с сервера
func (ac *APIClient) DownloadFile(fileID, outputPath string) error {
	url := fmt.Sprintf("%s/files/%s", ac.baseURL, fileID)

	resp, err := ac.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("не удалось отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("файл не найден")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("сервер вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	// Создаем выходной файл
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("не удалось создать выходной файл: %w", err)
	}
	defer outputFile.Close()

	// Копируем данные
	if _, err := io.Copy(outputFile, resp.Body); err != nil {
		return fmt.Errorf("не удалось записать данные в файл: %w", err)
	}

	return nil
}

// GetFileInfo получает информацию о файле
func (ac *APIClient) GetFileInfo(fileID string) (*chunking.FileMetadata, error) {
	url := fmt.Sprintf("%s/files/%s/info", ac.baseURL, fileID)

	resp, err := ac.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("не удалось отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("файл не найден")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("сервер вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	var metadata chunking.FileMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("не удалось десериализовать ответ: %w", err)
	}

	return &metadata, nil
}

// DeleteFile удаляет файл с сервера
func (ac *APIClient) DeleteFile(fileID string) error {
	url := fmt.Sprintf("%s/files/%s", ac.baseURL, fileID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("не удалось создать запрос: %w", err)
	}

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("не удалось отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("сервер вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ListFiles получает список всех файлов
func (ac *APIClient) ListFiles() ([]string, error) {
	url := fmt.Sprintf("%s/api/v1/files", ac.baseURL)

	resp, err := ac.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("не удалось отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("сервер вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	var files []string
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("не удалось десериализовать ответ: %w", err)
	}

	return files, nil
}

// HealthCheck проверяет доступность API сервера
func (ac *APIClient) HealthCheck() error {
	url := fmt.Sprintf("%s/health", ac.baseURL)

	resp, err := ac.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("сервер недоступен: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("сервер вернул код состояния %d", resp.StatusCode)
	}

	return nil
}
