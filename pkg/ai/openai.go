package ai

import (
    "bytes"
    "context"
    "encoding/csv"
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "os"
    "path/filepath"
    "strings"
    "time"

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
// 1. Init client
// 2. Create vector store named `vsName`
// 3. Upload all files (CSV → text if needed) into it
// 4. Create an assistant named `assistantName` using `model`
//    with File Search, Code Interpreter, Web Search, Image Gen tools
// 5. Return an *AI you can immediately call Chat() on.
func NewAI(ctx context.Context, model, systemPrompt, assistantName string, filePaths []string) (*AI, error) {
    // 0️⃣ Get API key
    apiKey := os.Getenv("OPENAI_API_KEY")
    if apiKey == "" {
        return nil, fmt.Errorf("OPENAI_API_KEY not set")
    }

    // 1️⃣ Init client
    client := openai.NewClient(option.WithAPIKey(apiKey))
    client.Config.AssistantVersion = "v2" // enable Assistants API v2

    // Prepare AI struct
    ai := &AI{client: &client, model: model}

    // 2️⃣ Create assistant with default tools
    log.Printf("Creating assistant %q with model %s", assistantName, model)
    asst, err := client.Beta.Assistants.New(ctx, openai.BetaAssistantNewParams{
        Name:         openai.String(assistantName),
        Model:        model,
        Instructions: openai.String(systemPrompt),
        Tools: []openai.AssistantToolUnionParam{
            {OfFileSearch: &openai.FileSearchToolParam{}},
            {OfCodeInterpreter: &openai.CodeInterpreterToolParam{}},
            {OfWebBrowser: &openai.WebBrowserToolParam{}},
            {OfImageGeneration: &openai.ImageGenerationToolParam{}},
        },
    })
    if err != nil {
        return nil, fmt.Errorf("assistant creation: %w", err)
    }
    log.Printf("🤖 Assistant %q created (ID=%s)", assistantName, asst.ID)
    ai.assistantID = asst.ID

    // 3️⃣ Create vector store
    vsName := fmt.Sprintf("store-%s-%s", model, assistantName)
    vs, err := client.VectorStores.New(ctx, openai.VectorStoreNewParams{Name: openai.String(vsName)})
    if err != nil {
        return nil, fmt.Errorf("vector store creation: %w", err)
    }
    log.Printf("🗄️  Vector store %q created (ID=%s)", vsName, vs.ID)
    ai.vectorStoreID = vs.ID

    // 4️⃣ Upload files
    var fileIDs []string
    for _, p := range filePaths {
        // convert CSV to text
        uploadName := filepath.Base(p)
        data, err := ioutil.ReadFile(p)
        if err != nil {
            return nil, fmt.Errorf("open %s: %w", p, err)
        }
        if strings.EqualFold(filepath.Ext(p), ".csv") {
            log.Printf("Converting CSV %q to plain text", p)
            data, err = csvToText(data)
            if err != nil {
                return nil, fmt.Errorf("csv→text %s: %w", p, err)
            }
            uploadName = strings.TrimSuffix(uploadName, ".csv") + ".txt"
        }

        log.Printf("📁 Uploading %s (size=%d)", uploadName, len(data))
        bf := bytes.NewReader(data)
        file, err := client.Files.Upload(ctx, openai.FileNewParams{
            Purpose: openai.FilePurposeAssistants,
            File:    bf,
            Name:    openai.String(uploadName),
        })
        if err != nil {
            return nil, fmt.Errorf("upload %s: %w", uploadName, err)
        }
        fileIDs = append(fileIDs, file.ID)
        log.Printf("   → file ID=%s", file.ID)
    }

    // Associate all uploaded files with the vector store
    _, err = client.VectorStores.Files.Add(ctx, ai.vectorStoreID, openai.VectorStoreFilesAddParams{
        FileIDs: fileIDs,
    })
    if err != nil {
        return nil, fmt.Errorf("add files to vector store: %w", err)
    }

    // Update assistant so File Search can use this VS
    _, err = client.Beta.Assistants.Update(ctx, openai.BetaAssistantUpdateParams{
        AssistantID: asst.ID,
        ToolResources: openai.BetaAssistantUpdateParamsToolResources{
            FileSearch: openai.BetaAssistantUpdateParamsToolResourcesFileSearch{
                VectorStoreIDs: []string{ai.vectorStoreID},
            },
        },
    })
    if err != nil {
        return nil, fmt.Errorf("assistant update: %w", err)
    }

    return ai, nil
}

// Chat sends a single user query and returns the assistant's reply.
func (ai *AI) Chat(ctx context.Context, question string) (string, error) {
    // 1️⃣ Create thread with user message
    thr, err := ai.client.Beta.Threads.New(ctx, openai.BetaThreadNewParams{
        Messages: []openai.BetaThreadNewParamsMessage{
            {
                Role: "user",
                Content: openai.BetaThreadNewParamsMessageContentUnion{
                    OfString: openai.String(question),
                },
            },
        },
    })
    if err != nil {
        return "", fmt.Errorf("create thread: %w", err)
    }

    // 2️⃣ Run assistant on thread
    run, err := ai.client.Beta.Threads.Runs.NewAndPoll(ctx, thr.ID, openai.BetaThreadRunNewParams{
        AssistantID: ai.assistantID,
    }, 0)
    if err != nil {
        return "", fmt.Errorf("assistant run: %w", err)
    }

    // 3️⃣ Retrieve assistant’s reply
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
    if strings.TrimSpace(resp) == "" {
        return "", fmt.Errorf("assistant returned empty response")
    }
    return resp, nil
}

// csvToText converts raw CSV bytes to plain text for better embeddings.
func csvToText(data []byte) ([]byte, error) {
    r := csv.NewReader(bytes.NewReader(data))
    var b strings.Builder
    for {
        record, err := r.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }
        b.WriteString(strings.Join(record, " "))
        b.WriteByte('\n')
    }
    return []byte(b.String()), nil
}
