package meeting

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultWhisperURL = "https://api.openai.com/v1/audio/transcriptions"

// Transcriber calls the OpenAI Whisper API and writes a Markdown transcript file.
type Transcriber struct {
	apiURL string
	apiKey string
	client *http.Client
}

// NewTranscriber creates a Transcriber using the real OpenAI endpoint.
// If apiKey is empty, os.Getenv("OPENAI_API_KEY") is used.
func NewTranscriber(apiKey string) *Transcriber {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	return NewTranscriberWithURL(defaultWhisperURL, apiKey)
}

// NewTranscriberWithURL creates a Transcriber against a custom URL (used in tests).
func NewTranscriberWithURL(apiURL, apiKey string) *Transcriber {
	return &Transcriber{
		apiURL: apiURL,
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Minute},
	}
}

// Transcribe sends wavPath to the Whisper API and writes the transcript as Markdown.
// Returns the path to the written .md file.
func (t *Transcriber) Transcribe(ctx context.Context, wavPath string, cfg Config) (string, error) {
	f, err := os.Open(wavPath)
	if err != nil {
		return "", fmt.Errorf("transcribe: open wav: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("model", cfg.Model); err != nil {
		return "", fmt.Errorf("transcribe: write model: %w", err)
	}
	if err := w.WriteField("response_format", "text"); err != nil {
		return "", fmt.Errorf("transcribe: write format: %w", err)
	}
	fw, err := w.CreateFormFile("file", filepath.Base(wavPath))
	if err != nil {
		return "", fmt.Errorf("transcribe: create form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return "", fmt.Errorf("transcribe: copy wav: %w", err)
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiURL, &buf)
	if err != nil {
		return "", fmt.Errorf("transcribe: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcribe: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("transcribe: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("transcribe: api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	transcript := strings.TrimSpace(string(body))
	mdPath := transcriptPath(cfg.OutputDir, wavPath)
	if err := writeMarkdown(mdPath, wavPath, transcript); err != nil {
		return "", fmt.Errorf("transcribe: write markdown: %w", err)
	}
	return mdPath, nil
}

func transcriptPath(outputDir, wavPath string) string {
	base := filepath.Base(wavPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(outputDir, "transcripts", name+".md")
}

func writeMarkdown(mdPath, wavPath, transcript string) error {
	content := fmt.Sprintf(
		"# Meeting Transcript\n\nSource: `%s`  \nDate: %s\n\n## Transcript\n\n%s\n",
		filepath.Base(wavPath),
		time.Now().Format("2006-01-02 15:04"),
		transcript,
	)
	return os.WriteFile(mdPath, []byte(content), 0o644)
}
