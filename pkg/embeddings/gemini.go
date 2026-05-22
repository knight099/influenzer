package embeddings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	GeminiEmbeddingAPIURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-2:embedContent"
)

type Part struct {
	Text string `json:"text"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type GeminiEmbeddingRequest struct {
	Content Content `json:"content"`
}

type EmbeddingValues struct {
	Values []float32 `json:"values"`
}

type GeminiEmbeddingResponse struct {
	Embedding EmbeddingValues `json:"embedding"`
}

type GeminiErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// Client represents a Gemini API client
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Gemini client
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetEmbedding generates a 768-dimension vector embedding for the given text
func (c *Client) GetEmbedding(text string) ([]float32, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("gemini api key is empty")
	}

	reqPayload := GeminiEmbeddingRequest{
		Content: Content{
			Parts: []Part{
				{Text: text},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", GeminiEmbeddingAPIURL, c.apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp GeminiErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("gemini API error (status %s, code %d): %s", errResp.Error.Status, errResp.Error.Code, errResp.Error.Message)
		}
		return nil, fmt.Errorf("gemini API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var respPayload GeminiEmbeddingResponse
	if err := json.Unmarshal(respBody, &respPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response payload: %w", err)
	}

	if len(respPayload.Embedding.Values) == 0 {
		return nil, fmt.Errorf("gemini API returned an empty embedding")
	}

	return respPayload.Embedding.Values, nil
}
