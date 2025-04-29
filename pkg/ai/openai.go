package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

// AI struct holds the assistant and thread context for interacting with OpenAI Assistants API.
type AI struct {
	assistantID  string       // OpenAI Assistant ID
	threadID     string       // Thread ID for conversation (persistent across chats)
	model        string       // Model used (default "gpt-4-turbo")
	instructions string       // System instructions for the assistant
	name         string       // Assistant name (if any)
	client       *http.Client // HTTP client for API requests
}

// NewAI initializes a new AI assistant with the given name, instructions, model, and optional files.
// It creates the assistant with all tools enabled (file search, code interpreter, web search, image generation)
// and starts a single persistent thread for conversation.
// The function returns an *AI instance or an error if initialization fails.
func NewAI(ctx context.Context, assistantName, instructions, model string, files []string) (*AI, error) {
	// Configure structured logging format and level
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetLevel(log.InfoLevel)

	// Use default model if none provided
	if model == "" {
		model = "gpt-4-turbo"
	}

	// Prepare HTTP client (with no timeout for streaming support)
	client := &http.Client{
		// We use the default transport; user can cancel via context
		Timeout: 0,
	}

	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Error("OPENAI_API_KEY environment variable is not set")
		return nil, errors.New("missing OpenAI API key")
	}

	// Initialize AI struct (threadID will be set after creating thread)
	ai := &AI{
		model:        model,
		instructions: instructions,
		name:         assistantName,
		client:       client,
	}
	// Set up base logger with assistant name for context
	baseLog := log.WithFields(log.Fields{
		"assistant_name": assistantName,
		"model":          model,
	})
	if instructions != "" {
		baseLog.WithField("instructions", instructions).Info("Initializing new AI assistant")
	} else {
		baseLog.Info("Initializing new AI assistant (no custom instructions provided)")
	}

	// Step 1: Upload files (if any) with purpose "assistants"
	fileIdMappedTools := map[string][]string{}
	for _, filePath := range files {
		baseLog := baseLog.WithField("file", filePath)
		f, err := os.Open(filePath)
		if err != nil {
			baseLog.WithError(err).Error("Failed to open file for upload")
			return nil, fmt.Errorf("open file %s: %w", filePath, err)
		}
		defer f.Close()

		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		// Write purpose field
		if err := writer.WriteField("purpose", "assistants"); err != nil {
			baseLog.WithError(err).Error("Failed to write multipart field 'purpose'")
			return nil, fmt.Errorf("write purpose field: %w", err)
		}
		// Create file field
		part, err := writer.CreateFormFile("file", filePath)
		if err != nil {
			baseLog.WithError(err).Error("Failed to create multipart file field")
			return nil, fmt.Errorf("create multipart file field: %w", err)
		}
		// Copy file content into form
		if _, err := io.Copy(part, f); err != nil {
			baseLog.WithError(err).Error("Failed to copy file content for upload")
			return nil, fmt.Errorf("copy file content: %w", err)
		}
		// Close multipart writer to finalize form data
		if err := writer.Close(); err != nil {
			baseLog.WithError(err).Error("Failed to close multipart writer")
			return nil, fmt.Errorf("close multipart writer: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/files", &b)
		if err != nil {
			baseLog.WithError(err).Error("Failed to create file upload request")
			return nil, fmt.Errorf("create file upload request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		// (No SSE here, normal JSON response)
		resp, err := client.Do(req)
		if err != nil {
			baseLog.WithError(err).Error("File upload request failed")
			return nil, fmt.Errorf("upload file request: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			baseLog.WithFields(log.Fields{
				"status": resp.Status,
				"body":   string(bodyBytes),
			}).Error("File upload failed")
			return nil, fmt.Errorf("file upload failed: status %s, body %s", resp.Status, string(bodyBytes))
		}
		var fileResp struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
			baseLog.WithError(err).Error("Failed to decode file upload response")
			return nil, fmt.Errorf("decode file upload: %w", err)
		}
		// check is file is csb then mapped to the perticular tool type
		if strings.Contains(strings.ToLower(filePath), "csv") {
			fileIdMappedTools["code_interpreter"] = append(fileIdMappedTools["code_interpreter"], fileResp.ID)
		}else {
			fileIdMappedTools["file_search"] = append(fileIdMappedTools["file_search"], fileResp.ID)
		}
		baseLog.WithField("file_id", fileResp.ID).Info("Uploaded file successfully")
	}

	body := map[string]interface{}{
		"name":     ai.name,
		"file_ids": fileIdMappedTools["file_search"],
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/vector_stores", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "assistants=v2")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := ai.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vector store creation failed: %s", msg)
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Step 2: Create the assistant with specified model, instructions, and tools
	assistantBody := map[string]interface{}{
		"model":        ai.model,
		"instructions": ai.instructions,
		"tools": []map[string]interface{}{
			{"type": "code_interpreter"},
			{"type": "file_search"},
		},
	}
	// So check the file is csv than add in the code_interpreter
	if len(fileIdMappedTools["code_interpreter"]) > 0 || len(fileIdMappedTools["file_search"]) > 0 {
		assistantBody["tool_resources"] = map[string]interface{}{
			"code_interpreter": map[string]interface{}{"file_ids": fileIdMappedTools["code_interpreter"]},
			"file_search": map[string]interface{}{"vector_store_ids": []string{out.ID}},
		}
	}

	bodyData, err := json.Marshal(assistantBody)
	if err != nil {
		baseLog.WithError(err).Error("Failed to marshal assistant creation body")
		return nil, fmt.Errorf("marshal assistant body: %w", err)
	}
	req, err = http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/assistants", bytes.NewReader(bodyData))
	if err != nil {
		baseLog.WithError(err).Error("Failed to create assistant request")
		return nil, fmt.Errorf("create assistant request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	// Include required beta header to use Assistants API (if needed for beta)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err = client.Do(req)
	if err != nil {
		baseLog.WithError(err).Error("Assistant creation request failed")
		return nil, fmt.Errorf("create assistant: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		baseLog.WithFields(log.Fields{
			"status": resp.Status,
			"body":   string(bodyBytes),
		}).Error("Assistant creation failed")
		return nil, fmt.Errorf("assistant creation failed: status %s, body %s", resp.Status, string(bodyBytes))
	}
	var asstResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&asstResp); err != nil {
		baseLog.WithError(err).Error("Failed to decode assistant creation response")
		return nil, fmt.Errorf("decode assistant response: %w", err)
	}
	ai.assistantID = asstResp.ID
	baseLog = baseLog.WithField("assistant_id", ai.assistantID)
	baseLog.Info("Created AI assistant successfully")

	// Step 3: Create a new conversation thread for this assistant
	threadReqBody := map[string]interface{}{
		// "assistant_id": ai.assistantID,
	}
	bodyData, _ = json.Marshal(threadReqBody)
	req, err = http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/threads", bytes.NewReader(bodyData))
	if err != nil {
		baseLog.WithError(err).Error("Failed to create thread request")
		return nil, fmt.Errorf("create thread request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err = client.Do(req)
	if err != nil {
		baseLog.WithError(err).Error("Thread creation request failed")
		return nil, fmt.Errorf("create thread: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		baseLog.WithFields(log.Fields{
			"status": resp.Status,
			"body":   string(bodyBytes),
		}).Error("Thread creation failed")
		return nil, fmt.Errorf("thread creation failed: status %s, body %s", resp.Status, string(bodyBytes))
	}
	var threadResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&threadResp); err != nil {
		baseLog.WithError(err).Error("Failed to decode thread creation response")
		return nil, fmt.Errorf("decode thread response: %w", err)
	}
	ai.threadID = threadResp.ID
	// Update baseLog to include thread context
	baseLog = baseLog.WithField("thread_id", ai.threadID)
	baseLog.Info("Conversation thread created successfully")

	return ai, nil
}

// Chat sends a user prompt to the AI assistant and returns the assistant's response as a markdown-formatted string.
// It maintains the conversation thread context across calls, streaming the assistant's response via SSE for real-time output.
// The function logs each step and tool usage for debugging and monitoring.
func (ai *AI) Chat(ctx context.Context, prompt string) (string, error) {
	if ai.assistantID == "" || ai.threadID == "" {
		log.Error("Assistant or thread not initialized properly")
		return "", errors.New("assistant not initialized")
	}
	// Base logger with context
	baseLog := log.WithFields(log.Fields{
		"assistant_id": ai.assistantID,
		"thread_id":    ai.threadID,
	})
	baseLog.WithField("user_prompt", prompt).Info("Sending user prompt to assistant")

	// Get API key from environment (should exist from initialization)
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		baseLog.Error("OPENAI_API_KEY is missing (was present during initialization?)")
		return "", errors.New("missing OpenAI API key")
	}

	// Step 1: Post the user message to the thread
	msgBody := map[string]interface{}{
		"role":    "user",
		"content": prompt,
	}
	bodyData, _ := json.Marshal(msgBody)
	msgURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/messages", ai.threadID)
	req, err := http.NewRequestWithContext(ctx, "POST", msgURL, bytes.NewReader(bodyData))
	if err != nil {
		baseLog.WithError(err).Error("Failed to create message request")
		return "", fmt.Errorf("create message request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err := ai.client.Do(req)
	if err != nil {
		baseLog.WithError(err).Error("Failed to send user message to thread")
		return "", fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		baseLog.WithFields(log.Fields{
			"status": resp.Status,
			"body":   string(bodyBytes),
		}).Error("Failed to add user message to thread")
		return "", fmt.Errorf("add message failed: status %s, body %s", resp.Status, string(bodyBytes))
	}
	// No need to parse message response; proceed to running the assistant

	// Step 2: Run the assistant and stream the response via SSE
	runURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/runs", ai.threadID)
	// We'll handle potentially multiple run phases if tools are invoked (function calls)
	finalAnswer := strings.Builder{}
	// For markdown formatting, track if we're in a code block when assembling output
	inCodeBlock := false

	// Container for function tool outputs (for function calling), and runID for continuation
	var toolOutputs []map[string]string
	var runID string

	// We'll loop until the run completes
	for {
		var runReqBody []byte
		if len(toolOutputs) == 0 {
			// First run request body
			runReqData := map[string]interface{}{
				"assistant_id": ai.assistantID,
				"stream":       true,
			}
			runReqBody, _ = json.Marshal(runReqData)
			baseLog.Info("Starting assistant run (streaming response)")
			req, err = http.NewRequestWithContext(ctx, "POST", runURL, bytes.NewReader(runReqBody))
		} else {
			// Submit tool outputs to continue the run
			contReqData := map[string]interface{}{
				"tool_outputs": toolOutputs,
				"stream":       true,
			}
			contReqBody, _ := json.Marshal(contReqData)
			if runID == "" {
				baseLog.Error("Missing run ID for tool output submission")
				return "", errors.New("invalid run continuation state")
			}
			contURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/runs/%s", ai.threadID, runID)
			baseLog.WithField("run_id", runID).Info("Submitting tool outputs and continuing run")
			req, err = http.NewRequestWithContext(ctx, "POST", contURL, bytes.NewReader(contReqBody))
			runReqBody = contReqBody
		}
		if err != nil {
			baseLog.WithError(err).Error("Failed to create run request")
			return "", fmt.Errorf("create run request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("OpenAI-Beta", "assistants=v2")

		resp, err := ai.client.Do(req)
		if err != nil {
			baseLog.WithError(err).Error("Run request failed")
			return "", fmt.Errorf("run request: %w", err)
		}
		// We need to read the SSE stream from resp.Body without closing it until done
		reader := resp.Body
		// Reset toolOutputs for next loop usage
		toolOutputs = nil

		// SSE parsing loop
		var eventName string
		var dataBuf strings.Builder
		_ = json.NewDecoder(reader)

		// We'll manually parse the SSE stream by reading line by line
		// Using a small buffer to accumulate partial lines if needed
		buf := make([]byte, 4096)
		var partialLine string
		doneStreaming := false

		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				chunk := partialLine + string(buf[:n])
				partialLine = "" // reset partial
				// Split chunk by line breaks to handle possibly multiple lines in one read
				lines := strings.Split(chunk, "\n")
				// If last line is not ended with newline, consider it partial for next iteration
				if !strings.HasSuffix(chunk, "\n") {
					partialLine = lines[len(lines)-1]
					lines = lines[:len(lines)-1]
				}
				for _, line := range lines {
					line = strings.TrimRight(line, "\r") // remove any \r
					if line == "" {
						// End of one SSE event
						if dataBuf.Len() == 0 && eventName == "" {
							// Just a keep-alive or empty event, ignore
							continue
						}
						// Parse the accumulated JSON data for this event
						var eventData interface{}
						dataStr := dataBuf.String()
						if dataStr == "[DONE]" {
							// End of stream sentinel (if any)
							doneStreaming = true
							break
						}
						if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
							baseLog.WithError(err).Warn("Failed to unmarshal SSE event data")
						}
						// Handle event based on eventName
						switch eventName {
						case "thread.message.delta":
							// This is a partial assistant message content
							// eventData is expected to be a JSON object with a "delta"
							// We'll decode it into a structured type for ease
							deltaBytes := []byte(dataStr)
							var msgDelta struct {
								Delta struct {
									Content []struct {
										Type string `json:"type"`
										Text *struct {
											Value string `json:"value"`
										} `json:"text,omitempty"`
										Code *struct {
											Language string `json:"language"`
											Content  string `json:"content"`
										} `json:"code,omitempty"`
										ImageFile *struct {
											FileID string `json:"file_id"`
										} `json:"image_file,omitempty"`
									} `json:"content"`
								} `json:"delta"`
							}
							if err := json.Unmarshal(deltaBytes, &msgDelta); err == nil {
								for _, part := range msgDelta.Delta.Content {
									switch part.Type {
									case "text":
										fmt.Println("-----> part.Text.Value", part.Text)
										if part.Text != nil {
											finalAnswer.WriteString(part.Text.Value)
										}
									case "code":
										// Ensure code is formatted in markdown
										if part.Code != nil {
											if !inCodeBlock {
												// Start a new code block
												finalAnswer.WriteString("\n```" + part.Code.Language + "\n")
												inCodeBlock = true
											}
											finalAnswer.WriteString(part.Code.Content)
											// We do NOT close code block here, might be continued in next delta
										}
									case "image_file":
										// If the assistant outputs an image, we can't directly display it without a path.
										// We log and add a placeholder in markdown.
										if part.ImageFile != nil {
											imageID := part.ImageFile.FileID
											baseLog.WithField("file_id", imageID).Info("Assistant generated an image file")
											// Add a Markdown placeholder for the image
											finalAnswer.WriteString(fmt.Sprintf("\n![generated image](image:%s)\n", imageID))
										}
									default:
										// other content types (e.g., 'logs' from code interpreter) can be handled if needed
									}
								}
							} else {
								// Fallback: if unmarshal fails, treat data as raw text
								fmt.Println("-----> dataStr", dataStr)
								finalAnswer.WriteString(dataStr)
							}
							// If we were in a code block and the next content is not code, we will close it
							// (We handle closing when the code block definitively ends on completed event or different part)
						case "thread.run.requires_action":
							// The assistant requires an action (likely function/tool output submission)
							baseLog.Info("Assistant requested tool/function action")
							reqBytes := []byte(dataStr)
							var reqAction struct {
								ID             string `json:"id"`
								Status         string `json:"status"`
								RequiredAction struct {
									Type       string `json:"type"`
									SubmitTool *struct {
										ToolCalls []struct {
											ID       string `json:"id"`
											Function struct {
												Name      string `json:"name"`
												Arguments string `json:"arguments"`
											} `json:"function"`
										} `json:"tool_calls"`
									} `json:"submit_tool_outputs,omitempty"`
								} `json:"required_action"`
							}
							if err := json.Unmarshal(reqBytes, &reqAction); err != nil {
								baseLog.WithError(err).Error("Failed to parse required_action event data")
								return "", fmt.Errorf("parse required_action: %w", err)
							}
							// We expect required_action.type == "submit_tool_outputs"
							if reqAction.RequiredAction.SubmitTool != nil {
								runID = reqAction.ID // the run ID that is waiting for tool outputs
								toolOutputs = []map[string]string{}
								// Handle each tool call that the assistant wants to use
								for _, toolCall := range reqAction.RequiredAction.SubmitTool.ToolCalls {
									fName := toolCall.Function.Name
									fArgs := toolCall.Function.Arguments
									baseLog := baseLog.WithField("tool", fName)
									baseLog.Infof("Assistant invoked tool: %s with args: %s", fName, fArgs)
									var output interface{}
									// Parse the function arguments JSON
									var argsMap map[string]interface{}
									_ = json.Unmarshal([]byte(fArgs), &argsMap)
									// Execute the corresponding tool
									switch fName {
									// case "web_search":
									// 	// Web search tool: perform a web search for the query and return a summary.
									// 	query, _ := argsMap["query"].(string)
									// 	if query == "" {
									// 		output = map[string]interface{}{
									// 			"error": "missing query",
									// 		}
									// 		baseLog.Warn("Web search called with empty query")
									// 	} else {
									// 		baseLog.Info("Performing web search for query")
									// 		// *** Placeholder for actual web search integration ***
									// 		// For demonstration, we simulate search results.
									// 		simulatedResult := fmt.Sprintf("Top result for '%s': Example result snippet.", query)
									// 		output = map[string]interface{}{
									// 			"results": []string{simulatedResult},
									// 		}
									// 		baseLog.Info("Web search completed (simulated result)")
									// 	}
									// case "generate_image":
									// 	// Image generation tool: use OpenAI Image API to generate an image and return file ID.
									// 	prompt, _ := argsMap["prompt"].(string)
									// 	if prompt == "" {
									// 		output = map[string]interface{}{
									// 			"error": "missing prompt",
									// 		}
									// 		baseLog.Warn("Image generation called with empty prompt")
									// 	} else {
									// 		baseLog.Info("Generating image via OpenAI Images API")
									// 		imgReqBody, _ := json.Marshal(map[string]interface{}{
									// 			"prompt": prompt,
									// 			"n":      1,
									// 			"size":   "512x512",
									// 		})
									// 		imgReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/images/generations", bytes.NewReader(imgReqBody))
									// 		if err != nil {
									// 			baseLog.WithError(err).Error("Failed to create image generation request")
									// 			output = map[string]interface{}{
									// 				"error": "image generation request failed",
									// 			}
									// 		} else {
									// 			imgReq.Header.Set("Authorization", "Bearer "+apiKey)
									// 			imgReq.Header.Set("Content-Type", "application/json")
									// 			imgResp, err := ai.client.Do(imgReq)
									// 			if err != nil || imgResp.StatusCode != http.StatusOK {
									// 				if err != nil {
									// 					baseLog.WithError(err).Error("Image generation API call failed")
									// 				} else {
									// 					bodyBytes, _ := io.ReadAll(imgResp.Body)
									// 					baseLog.WithFields(log.Fields{
									// 						"status": imgResp.Status,
									// 						"body":   string(bodyBytes),
									// 					}).Error("Image generation API returned error")
									// 				}
									// 				output = map[string]interface{}{
									// 					"error": "image generation failed",
									// 				}
									// 			} else {
									// 				var imgRespBody struct {
									// 					Data []struct {
									// 						URL string `json:"url"`
									// 					} `json:"data"`
									// 				}
									// 				_ = json.NewDecoder(imgResp.Body).Decode(&imgRespBody)
									// 				imgResp.Body.Close()
									// 				if len(imgRespBody.Data) > 0 {
									// 					imageURL := imgRespBody.Data[0].URL
									// 					// Download the image data
									// 					imgDataResp, err := ai.client.Get(imageURL)
									// 					if err != nil {
									// 						baseLog.WithError(err).Error("Failed to download generated image")
									// 						output = map[string]interface{}{
									// 							"error": "failed to download image",
									// 						}
									// 					} else {
									// 						defer imgDataResp.Body.Close()
									// 						// Upload this image as a file to OpenAI (purpose: assistants)
									// 						var imgBuf bytes.Buffer
									// 						imgWriter := multipart.NewWriter(&imgBuf)
									// 						_ = imgWriter.WriteField("purpose", "assistants")
									// 						imgPart, _ := imgWriter.CreateFormFile("file", "generated_image.png")
									// 						_, _ = io.Copy(imgPart, imgDataResp.Body)
									// 						_ = imgWriter.Close()
									// 						upReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/files", &imgBuf)
									// 						if err != nil {
									// 							baseLog.WithError(err).Error("Failed to create image upload request")
									// 							output = map[string]interface{}{
									// 								"error": "image upload request failed",
									// 							}
									// 						} else {
									// 							upReq.Header.Set("Authorization", "Bearer "+apiKey)
									// 							upReq.Header.Set("Content-Type", imgWriter.FormDataContentType())
									// 							upResp, err := ai.client.Do(upReq)
									// 							if err != nil {
									// 								baseLog.WithError(err).Error("Image upload failed")
									// 								output = map[string]interface{}{
									// 									"error": "image upload failed",
									// 								}
									// 							} else {
									// 								defer upResp.Body.Close()
									// 								if upResp.StatusCode != http.StatusOK {
									// 									bodyBytes, _ := io.ReadAll(upResp.Body)
									// 									baseLog.WithFields(log.Fields{
									// 										"status": upResp.Status,
									// 										"body":   string(bodyBytes),
									// 									}).Error("Image upload API returned error")
									// 									output = map[string]interface{}{
									// 										"error": "image upload error",
									// 									}
									// 								} else {
									// 									var fileResp struct {
									// 										ID string `json:"id"`
									// 									}
									// 									_ = json.NewDecoder(upResp.Body).Decode(&fileResp)
									// 									baseLog.WithField("file_id", fileResp.ID).Info("Generated image uploaded as file")
									// 									// Return the file ID in output
									// 									output = map[string]interface{}{
									// 										"image_file_id": fileResp.ID,
									// 									}
									// 								}
									// 							}
									// 						}
									// 					}
									// 				} else {
									// 					baseLog.Error("Image generation API returned no data")
									// 					output = map[string]interface{}{
									// 						"error": "no image generated",
									// 					}
									// 				}
									// 			}
									// 		}
									// 	}
									default:
										// Unknown tool/function – return an error in output
										baseLog.Warn("Unknown tool requested: ", fName)
										output = map[string]interface{}{
											"error": fmt.Sprintf("unknown tool %s", fName),
										}
									}
									// Prepare tool output as JSON string
									outputJSON, _ := json.Marshal(output)
									toolOutputs = append(toolOutputs, map[string]string{
										"tool_call_id": toolCall.ID,
										"output":       string(outputJSON),
									})
								}
							}
							// Break out of SSE read loop to send tool outputs and continue the run
							doneStreaming = true
						case "thread.run.completed":
							// The assistant run is completed
							baseLog.Info("Assistant run completed")
							// Close any unclosed code block in final answer
							if inCodeBlock {
								finalAnswer.WriteString("\n```\n")
								inCodeBlock = false
							}
							doneStreaming = true
						}
						// Reset buffer and eventName for next event
						eventName = ""
						dataBuf.Reset()
					} else if strings.HasPrefix(line, "data:") {
						// Accumulate data lines (strip "data: ")
						dataLine := strings.TrimPrefix(line, "data:")
						// Remove leading space if present
						if len(dataLine) > 0 && dataLine[0] == ' ' {
							dataLine = dataLine[1:]
						}
						dataBuf.WriteString(dataLine)
					} else if strings.HasPrefix(line, "event:") {
						// Set the current event name (strip "event: ")
						eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
					} else if strings.HasPrefix(line, ":") {
						// SSE comment or heartbeat, ignore
						continue
					}
				}
			}
			if doneStreaming {
				// If we've completed processing this run (either finished or need to handle tool), break out
				break
			}
			if readErr != nil {
				// If error reading the stream:
				if readErr == io.EOF {
					// Stream ended normally
					break
				}
				// Other errors
				baseLog.WithError(readErr).Error("Error reading SSE stream")
				return "", fmt.Errorf("read stream: %w", readErr)
			}
		}
		resp.Body.Close()

		if toolOutputs != nil && len(toolOutputs) > 0 {
			// We have tool outputs to submit, loop will iterate again to continue the run
			continue
		} else {
			// No tool outputs pending, means run is completed with final answer
			break
		}
	}

	assistantResponse := finalAnswer.String()
	baseLog.WithField("assistant_response", assistantResponse).Info("Received assistant response")
	// The final answer is already formatted in Markdown as we constructed (code blocks, etc.)
	return assistantResponse, nil
}
