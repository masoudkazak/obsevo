package services_test

import (
	"testing"

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
