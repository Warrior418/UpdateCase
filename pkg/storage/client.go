package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"TestCase/pkg/chunking"
)

// StorageClient представляет клиент для взаимодействия с сервером хранения
type StorageClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewStorageClient создает новый клиент для сервера хранения
func NewStorageClient(baseURL string) *StorageClient {
	return &StorageClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// StoreChunk сохраняет кусок файла на сервере хранения
func (c *StorageClient) StoreChunk(chunk *chunking.FileChunk) error {
	data, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("не удалось сериализовать кусок: %w", err)
	}

	resp, err := c.HTTPClient.Post(
		fmt.Sprintf("%s/api/v1/chunks", c.BaseURL),
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("не удалось отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("сервер вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetChunk получает кусок файла с сервера хранения
func (c *StorageClient) GetChunk(chunkID string) (*chunking.FileChunk, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/api/v1/chunks/%s", c.BaseURL, chunkID))
	if err != nil {
		return nil, fmt.Errorf("не удалось отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("сервер вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	var chunk chunking.FileChunk
	if err := json.NewDecoder(resp.Body).Decode(&chunk); err != nil {
		return nil, fmt.Errorf("не удалось декодировать ответ: %w", err)
	}

	return &chunk, nil
}

// DeleteChunk удаляет кусок файла с сервера хранения
func (c *StorageClient) DeleteChunk(chunkID string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/chunks/%s", c.BaseURL, chunkID), nil)
	if err != nil {
		return fmt.Errorf("не удалось создать запрос: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
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

// HealthCheck проверяет состояние сервера хранения
func (c *StorageClient) HealthCheck() error {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/health", c.BaseURL))
	if err != nil {
		return fmt.Errorf("не удалось подключиться к серверу: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("сервер вернул статус %d", resp.StatusCode)
	}

	return nil
}

// GetInfo получает информацию о сервере хранения
func (c *StorageClient) GetInfo() (map[string]interface{}, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/api/v1/info", c.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("не удалось отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("сервер вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("не удалось декодировать ответ: %w", err)
	}

	return info, nil
}
