package agentkit

import (
	"context"
	"strings"
	"testing"
	"time"

	aggoagent "github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/CoolBanHub/aggo/memory/builtin/storage"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type memoryAnalyzerModel struct{}

func (memoryAnalyzerModel) Generate(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.AgenticMessage, error) {
	return assistantMessage(`{"op":"update","memory":"用户叫 Alice，喜欢摄影。"}`), nil
}

func (m memoryAnalyzerModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	reader, writer := schema.Pipe[*schema.AgenticMessage](1)
	go func() {
		writer.Send(m.mustGenerate(ctx, input, opts...), nil)
		writer.Close()
	}()
	return reader, nil
}

func (memoryAnalyzerModel) mustGenerate(context.Context, []*schema.AgenticMessage, ...model.Option) *schema.AgenticMessage {
	return assistantMessage(`{"op":"update","memory":"用户叫 Alice，喜欢摄影。"}`)
}

func TestBuiltinMemoryProviderMemorizeAndRetrieve(t *testing.T) {
	debounce := 0
	cfg := builtin.DefaultMemoryConfig()
	cfg.EnableSessionSummary = false
	cfg.DebounceWindowSeconds = &debounce
	cfg.AsyncWorkerPoolSize = 1

	provider, err := memory.GlobalRegistry().CreateProvider("builtin", &builtin.ProviderConfig{
		ChatModel:    memoryAnalyzerModel{},
		Storage:      storage.NewMemoryStore(),
		MemoryConfig: cfg,
	})
	if err != nil {
		t.Fatalf("create memory provider: %v", err)
	}
	defer provider.Close()

	err = provider.Memorize(context.Background(), &memory.MemorizeRequest{
		UserID:    "42",
		SessionID: "session-1",
		Messages: []*schema.AgenticMessage{
			schema.UserAgenticMessage("我叫 Alice，喜欢摄影"),
			assistantMessage("好的，我记住了"),
		},
	})
	if err != nil {
		t.Fatalf("memorize: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		result, retrieveErr := provider.Retrieve(context.Background(), &memory.RetrieveRequest{
			UserID:    "42",
			SessionID: "session-1",
			Limit:     20,
		})
		if retrieveErr == nil {
			for _, msg := range result.ContextMessages {
				if strings.Contains(messageText(msg), "Alice") {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("memory was not available through Retrieve after Memorize")
}

func TestRunWithBuiltinMemoryProviderUsesIdentityContext(t *testing.T) {
	debounce := 0
	cfg := builtin.DefaultMemoryConfig()
	cfg.EnableSessionSummary = false
	cfg.DebounceWindowSeconds = &debounce
	cfg.AsyncWorkerPoolSize = 1

	provider, err := memory.GlobalRegistry().CreateProvider("builtin", &builtin.ProviderConfig{
		ChatModel:    memoryAnalyzerModel{},
		Storage:      storage.NewMemoryStore(),
		MemoryConfig: cfg,
	})
	if err != nil {
		t.Fatalf("create memory provider: %v", err)
	}
	defer provider.Close()

	agent, err := aggoagent.NewAgentBuilder(memoryAnalyzerModel{}).
		WithMemory(provider).
		Build(context.Background())
	if err != nil {
		t.Fatalf("build agent: %v", err)
	}
	if _, err := Run(context.Background(), agent, nil, &Input{Text: "我叫 Alice，喜欢摄影"}, nil, nil, "42", "session-1"); err != nil {
		t.Fatalf("run agent: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		result, retrieveErr := provider.Retrieve(context.Background(), &memory.RetrieveRequest{
			UserID: "42", SessionID: "session-1", Limit: 20,
		})
		if retrieveErr == nil {
			for _, msg := range result.ContextMessages {
				if strings.Contains(messageText(msg), "Alice") {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent run did not memorize data for the supplied identity")
}

func assistantMessage(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: text}),
		},
	}
}

func messageText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range msg.ContentBlocks {
		if block == nil {
			continue
		}
		if block.UserInputText != nil {
			b.WriteString(block.UserInputText.Text)
		}
		if block.AssistantGenText != nil {
			b.WriteString(block.AssistantGenText.Text)
		}
	}
	return b.String()
}
