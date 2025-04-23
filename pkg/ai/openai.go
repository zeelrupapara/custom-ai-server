package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

const baseURL = "https://api.openai.com/v1"

// AI wraps OpenAI Assistants API interactions via native HTTP calls.
type AI struct {
	APIKey      string
	httpClient  *http.Client
	assistantID string
	threadID    string
}

// NewAI initializes an AI client, uploads files, creates a vector store, and creates an assistant.
// It returns an *AI with assistantID set, ready for chat.
func NewAI(ctx context.Context, assistantName, instructions, model string, files []string) (*AI, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	ai := &AI{APIKey: key, httpClient: client}

	// 1. Upload files and collect file IDs
	var fileIDs []string
	for _, path := range files {
		id, err := ai.uploadFile(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("upload file %s: %w", path, err)
		}
		fileIDs = append(fileIDs, id)
	}

	// 2. Create vector store if any files uploaded
	var vsID string
	if len(fileIDs) > 0 {
		id, err := ai.createVectorStore(ctx, "assistant-vectorstore", fileIDs)
		if err != nil {
			return nil, fmt.Errorf("create vector store: %w", err)
		}
		vsID = id
	}

	// 3. Create assistant with all tools enabled
	asstID, err := ai.createAssistant(ctx, assistantName, instructions, model, vsID)
	if err != nil {
		return nil, fmt.Errorf("create assistant: %w", err)
	}
	ai.assistantID = asstID
	return ai, nil
}

// Chat sends a user prompt and streams the assistant's response via stdout.
func (ai *AI) Chat(ctx context.Context, prompt string) (string, error) {
	// … send the user message …
	reply, err := ai.runAssistant(ctx, prompt)
	if err != nil {
		return "", err
	}
	return reply, nil
}

// uploadFile uploads a file (converts CSV to text) and returns the file ID.
func (ai *AI) uploadFile(ctx context.Context, filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	reader := io.Reader(f)
	filename := filePath
	// CSV conversion
	if strings.HasSuffix(strings.ToLower(filePath), ".csv") {
		buf := &bytes.Buffer{}
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := s.Text()
			buf.WriteString(strings.ReplaceAll(line, ",", "\t") + "\n")
		}
		if err := s.Err(); err != nil {
			return "", err
		}
		reader = buf
		filename = filename + ".txt"
	}

	// multipart form
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", filename)
	io.Copy(fw, reader)
	mw.WriteField("purpose", "assistants")
	mw.Close()

	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/files", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	ai.addAuth(req)

	resp, err := ai.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("file upload failed: %s", msg)
	}

	var out struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	return out.ID, nil
}

// createVectorStore creates a vector store from file IDs.
func (ai *AI) createVectorStore(ctx context.Context, name string, fileIDs []string) (string, error) {
	body := map[string]interface{}{"name": name, "file_ids": fileIDs}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/vector_stores", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "assistants=v2")
	ai.addAuth(req)

	resp, err := ai.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("vector store failed: %s", msg)
	}

	var out struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	return out.ID, nil
}

// createAssistant creates an assistant with tools and optional vector store.
func (ai *AI) createAssistant(ctx context.Context, name, instr, model, vsID string) (string, error) {
	tools := []map[string]string{
		{"type": "file_search"},
		{"type": "code_interpreter"},
		// {"type": "web_search"},
		// {"type": "image_generation"},
	}

	body := map[string]interface{}{
		"name":         name,
		"instructions": instr,
		"model":        model,
		"tools":        tools,
		"tool_resources": map[string]interface{}{
			"file_search": map[string]interface{}{
				"vector_store_ids": []string{
					vsID,
				},
			},
		},
	}
	// if vsID != "" {
	// 	body["vector_store_ids"] = []string{vsID}
	// }
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/assistants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "assistants=v2")
	ai.addAuth(req)

	resp, err := ai.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("assistant creation failed: %s", msg)
	}
	var out struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	return out.ID, nil
}

// createThread starts a new conversation thread.
func (ai *AI) createThread(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/threads", nil)
	ai.addAuth(req)

	resp, err := ai.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("thread creation failed: %s", msg)
	}
	var out struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	return out.ID, nil
}

// sendUserMessage posts user input to the thread.
func (ai *AI) sendUserMessage(ctx context.Context, text string) error {
	body := map[string]interface{}{"role": "user", "content": text}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/threads/"+ai.threadID+"/messages", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	ai.addAuth(req)

	resp, err := ai.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send message failed: %s", msg)
	}
	return nil
}

// runAssistant runs the assistant on the thread and streams SSE events.
func (ai *AI) runAssistant(ctx context.Context, prompt string) (string, error) {
	// Prepare the request body to trigger the assistant
	body := map[string]string{
		"assistant_id": ai.assistantID, // The assistant ID used to process the request
	}
	b, _ := json.Marshal(body)

	// Create the request for the assistant run
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/threads/"+ai.threadID+"/runs", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream") // Requesting an SSE stream
	req.Header.Set("OpenAI-Beta", "assistants=v2")
	ai.addAuth(req)                               // Add the necessary Authorization header

	// Send the request
	resp, err := ai.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Set up a buffer to read the SSE stream response
	reader := bufio.NewReader(resp.Body)
	var answer strings.Builder

	// Read the response line by line
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break // End of the stream, exit loop
			}
			return "", fmt.Errorf("read error: %w", err)
		}

		// Look for data: lines in the SSE stream
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Parse the data (expected JSON content)
		data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if data == "[DONE]" {
			break // Stream completed, exit loop
		}

		var evt struct {
			Type  string `json:"type"`
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			// If JSON parsing fails, continue to the next event
			continue
		}

		// Append the content from delta to the final answer
		if evt.Type == "delta" && evt.Delta.Content != "" {
			answer.WriteString(evt.Delta.Content)
		}
	}

	// Return the full answer accumulated from the stream
	return answer.String(), nil
}

// addAuth attaches the Authorization header.
func (ai *AI) addAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+ai.APIKey)
}
