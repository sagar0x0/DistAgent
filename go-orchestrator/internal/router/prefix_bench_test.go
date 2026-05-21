package router

import (
	"fmt"
	"strings"
	"testing"

	distagentv1 "github.com/sagar/distagent/gen/distagent/v1"
)

func generateTools(count int) []*distagentv1.ToolDefinition {
	tools := make([]*distagentv1.ToolDefinition, count)
	for i := 0; i < count; i++ {
		tools[i] = &distagentv1.ToolDefinition{
			Name:                 fmt.Sprintf("tool_%d", i),
			Description:          fmt.Sprintf("This is the description for tool %d which does some very useful operation in the system.", i),
			ParametersJsonSchema: `{"type":"object", "properties": {"arg1": {"type": "string"}}}`,
		}
	}
	return tools
}

// BenchmarkComputePrefixHash tests the pure SHA-256 computation speed as tool schemas scale
func BenchmarkComputePrefixHash(b *testing.B) {
	scales := []int{0, 2, 10, 50}
	prompt := "You are a helpful AI assistant. Always use tools to verify facts."

	for _, scale := range scales {
		b.Run(fmt.Sprintf("%d_Tools", scale), func(b *testing.B) {
			tools := generateTools(scale)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = ComputePrefixHash(prompt, tools)
			}
		})
	}
}

// BenchmarkComputePrefixHash_LargePrompt tests computation with a realistic ~4KB prompt
func BenchmarkComputePrefixHash_LargePrompt(b *testing.B) {
	promptLines := make([]string, 100)
	for i := 0; i < 100; i++ {
		promptLines[i] = "This is a line of text in the system prompt representing some deeply detailed instruction or persona definition."
	}
	prompt := strings.Join(promptLines, "\n")
	tools := generateTools(10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ComputePrefixHash(prompt, tools)
	}
}
