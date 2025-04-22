package ai

import (
	"context"
	"fmt"
	"log"
	"os"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// AI wraps OpenAI client + our assistant resources.
type AI struct {
	client        *openai.Client
	model         string
	vectorStoreID string
	assistantID   string
}

// NewAI will:
// 1. Create a vector store named `vsName`
// 2. Upload all files in filePaths into it
// 3. Create an assistant named `assistantName` using `model`
// 4. Return an *AI you can immediately call Chat() on.
func NewAI(ctx context.Context, model, systemPrompt string, assistantName string, filePaths []string) (*AI, error) {
	// 0️⃣ Get API key from env var
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not set")
	}

	vsName := fmt.Sprintf("store-%s-%s", model, assistantName)
	// 1️⃣ Init client
	client := openai.NewClient(option.WithAPIKey(apiKey))

	// 2️⃣ Create vector store
	vs, err := client.VectorStores.New(ctx, openai.VectorStoreNewParams{
		Name: openai.String(vsName),
	})
	if err != nil {
		return nil, fmt.Errorf("vector store creation: %w", err)
	}
	log.Printf("🗄️  Vector store %q created (ID=%s)", vsName, vs.ID)

	// 3️⃣ Upload files into vector store
	for _, p := range filePaths {
		// check file csv or not
		// if filepath.Ext(p) == ".csv" {
		// 	p, err = utils.CSVToText(p)
		// 	if err != nil {
		// 		return nil, fmt.Errorf("csv to text: %w", err)
		// 	}
		// }
		fmt.Println("📁 Uploading", p)
		f, err := os.Open(p)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", p, err)
		}
		defer f.Close()

		_, err = client.VectorStores.Files.UploadAndPoll(ctx, vs.ID, openai.FileNewParams{
			Purpose: openai.FilePurposeAssistants,
			File:    f,
		}, 0)
		if err != nil {
			return nil, fmt.Errorf("upload %s: %w", p, err)
		}
		log.Printf("📁 Uploaded %s", p)
	}

	// 4️⃣ Create assistant with file_search tool
	asst, err := client.Beta.Assistants.New(ctx, openai.BetaAssistantNewParams{
		Name:         openai.String(assistantName),
		Model:        model,
		Instructions: openai.String(systemPrompt),
		Tools: []openai.AssistantToolUnionParam{
			{OfFileSearch: &openai.FileSearchToolParam{}},
		},
		ToolResources: openai.BetaAssistantNewParamsToolResources{
			FileSearch: openai.BetaAssistantNewParamsToolResourcesFileSearch{
				VectorStoreIDs: []string{vs.ID},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("assistant creation: %w", err)
	}
	log.Printf("🤖 Assistant %q created (ID=%s)", assistantName, asst.ID)

	return &AI{
		client:        &client,
		model:         model,
		vectorStoreID: vs.ID,
		assistantID:   asst.ID,
	}, nil
}

// Chat sends a single user query and returns the assistant's reply (statelessly).
func (ai *AI) Chat(ctx context.Context, question string) (string, error) {
	// 1️⃣ Start a new thread with the user question
	thr, err := ai.client.Beta.Threads.New(ctx, openai.BetaThreadNewParams{
		Messages: []openai.BetaThreadNewParamsMessage{
			{Role: "user", Content: openai.BetaThreadNewParamsMessageContentUnion{
				OfString: openai.String(question),
			}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create thread: %w", err)
	}

	// 2️⃣ Run the assistant on that thread (poll until done)
	_, err = ai.client.Beta.Threads.Runs.NewAndPoll(ctx, thr.ID, openai.BetaThreadRunNewParams{
		AssistantID: ai.assistantID,
	}, 0)
	if err != nil {
		return "", fmt.Errorf("assistant run: %w", err)
	}

	// 3️⃣ Fetch the messages and extract assistant’s text
	page, err := ai.client.Beta.Threads.Messages.List(ctx, thr.ID, openai.BetaThreadMessageListParams{})
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}

	var resp string
	for _, m := range page.Data {
		if m.Role == "assistant" {
			for _, c := range m.Content {
				if c.Type == "text" {
					resp += c.Text.Value
				}
			}
		}
	}
	return resp, nil
}
