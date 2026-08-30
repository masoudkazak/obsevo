package services_test

import (
	"encoding/json"
	"testing"

	"github.com/langfuse-light/langfuse-light/internal/db"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

func TestCompileTemplate_NoVariables(t *testing.T) {
	svc := services.NewPromptService(nil)
	result, err := svc.CompileTemplate("Hello world", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", result)
	}
}

func TestCompileTemplate_WithVariables(t *testing.T) {
	svc := services.NewPromptService(nil)
	result, err := svc.CompileTemplate("Hello {{name}}, you are {{age}} years old", map[string]string{
		"name": "Alice",
		"age":  "30",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Hello Alice, you are 30 years old"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestCompileTemplate_UndefinedVariable(t *testing.T) {
	svc := services.NewPromptService(nil)
	_, err := svc.CompileTemplate("Hello {{name}}", map[string]string{})
	if err == nil {
		t.Fatal("expected error for undefined variable, got nil")
	}
}

func TestCompileTemplate_MultipleSameVariable(t *testing.T) {
	svc := services.NewPromptService(nil)
	result, err := svc.CompileTemplate("{{x}} and {{x}}", map[string]string{
		"x": "same",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "same and same" {
		t.Errorf("expected 'same and same', got %q", result)
	}
}

func TestExtractTemplateVariables(t *testing.T) {
	svc := services.NewPromptService(nil)
	vars := svc.ExtractTemplateVariables("Hello {{name}}, you are {{age}} years old, {{name}}")
	if len(vars) != 2 {
		t.Fatalf("expected 2 variables, got %d: %v", len(vars), vars)
	}
	if vars[0] != "name" || vars[1] != "age" {
		t.Errorf("expected [name, age], got %v", vars)
	}
}

func TestExtractTemplateVariables_None(t *testing.T) {
	svc := services.NewPromptService(nil)
	vars := svc.ExtractTemplateVariables("no variables here")
	if len(vars) != 0 {
		t.Errorf("expected 0 variables, got %d: %v", len(vars), vars)
	}
}

// --- prompt body encoding and label resolution -------------------------------

// promptRow builds a stored prompt row for Describe().
func promptRow(promptType, body string) db.Prompt {
	return db.Prompt{Type: promptType, Prompt: json.RawMessage(body)}
}

func TestDescribeTextPrompt(t *testing.T) {
	service := services.NewPromptService(nil)

	described := service.Describe(promptRow("text", `"Hello {{name}}, welcome to {{product}}!"`))

	if described.PromptText != "Hello {{name}}, welcome to {{product}}!" {
		t.Errorf("expected the decoded template, got %q", described.PromptText)
	}
	if len(described.Variables) != 2 ||
		described.Variables[0] != "name" || described.Variables[1] != "product" {
		t.Errorf("expected [name product] in order of appearance, got %v", described.Variables)
	}
	if len(described.ChatMessages) != 0 {
		t.Error("expected no chat messages for a text prompt")
	}
}

func TestDescribeChatPrompt(t *testing.T) {
	service := services.NewPromptService(nil)

	body := `[{"role":"system","content":"You are {{persona}}."},{"role":"user","content":"{{question}}"}]`
	described := service.Describe(promptRow("chat", body))

	if len(described.ChatMessages) != 2 {
		t.Fatalf("expected 2 chat messages, got %d", len(described.ChatMessages))
	}
	if described.ChatMessages[0].Role != "system" {
		t.Errorf("expected the first role to be system, got %q", described.ChatMessages[0].Role)
	}
	// Variables are collected across every message, so a template is complete
	// only when all of them are supplied.
	if len(described.Variables) != 2 {
		t.Errorf("expected variables from both messages, got %v", described.Variables)
	}
}

func TestCompileTemplateToleratesInnerWhitespace(t *testing.T) {
	service := services.NewPromptService(nil)

	compiled, err := service.CompileTemplate("Hi {{ name }}!", map[string]string{"name": "Ada"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compiled != "Hi Ada!" {
		t.Errorf("expected whitespace inside the placeholder to be tolerated, got %q", compiled)
	}
}

func TestCompileTemplateRepeatsAVariable(t *testing.T) {
	service := services.NewPromptService(nil)

	compiled, err := service.CompileTemplate("{{x}} and {{x}}", map[string]string{"x": "y"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compiled != "y and y" {
		t.Errorf("expected every occurrence to be substituted, got %q", compiled)
	}
}
