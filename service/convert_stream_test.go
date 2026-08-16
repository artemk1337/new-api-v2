package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestStreamResponseOpenAI2ClaudeToolUseBlockSequence(t *testing.T) {
	tests := []struct {
		name       string
		chunks     []dto.ChatCompletionsStreamResponse
		toolStarts int
	}{
		{
			name: "single tool call",
			chunks: []dto.ChatCompletionsStreamResponse{
				toolChunk(toolCall(0, "call-1", "lookup", "")),
				toolChunk(toolCallArgs(0, `{"q":"x"}`)),
				finishToolChunk(),
			},
			toolStarts: 1,
		},
		{
			name: "one based index after text",
			chunks: []dto.ChatCompletionsStreamResponse{
				textChunk("before"),
				toolChunk(toolCall(1, "call-1", "lookup", "")),
				finishToolChunk(),
			},
			toolStarts: 1,
		},
		{
			name: "parallel calls in first chunk",
			chunks: []dto.ChatCompletionsStreamResponse{
				{
					Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ToolCalls: []dto.ToolCallResponse{
							toolCall(0, "call-1", "lookup", ""),
							toolCall(1, "call-2", "inspect", ""),
						},
					}}},
				},
				{
					Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ToolCalls: []dto.ToolCallResponse{
							toolCallArgs(0, `{"q":"x"}`),
							toolCallArgs(1, `{"path":"/tmp"}`),
						},
					}}},
				},
				finishToolChunk(),
			},
			toolStarts: 2,
		},
		{
			name: "parallel calls without upstream indexes",
			chunks: []dto.ChatCompletionsStreamResponse{
				{
					Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"}}},
				},
				toolChunk(toolCallWithoutIndex("call-1", "lookup")),
				toolChunk(toolCallWithoutIndex("call-2", "inspect")),
				finishToolChunk(),
			},
			toolStarts: 2,
		},
		{
			name: "metadata-only tool followed by text",
			chunks: []dto.ChatCompletionsStreamResponse{
				toolChunk(toolCall(0, "call-1", "", "")),
				textChunk("after"),
				finishToolChunk(),
			},
			toolStarts: 0,
		},
		{
			name: "metadata-only tool followed by thinking",
			chunks: []dto.ChatCompletionsStreamResponse{
				toolChunk(toolCall(0, "call-1", "", "")),
				thinkingChunk("after"),
				finishToolChunk(),
			},
			toolStarts: 0,
		},
		{
			name: "metadata-only tool completes later",
			chunks: []dto.ChatCompletionsStreamResponse{
				toolChunk(toolCall(0, "call-1", "", "")),
				toolChunk(toolCall(0, "call-1", "lookup", "")),
				toolChunk(toolCall(0, "call-1", "", `{"q":"x"}`)),
				finishToolChunk(),
			},
			toolStarts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
			}

			var responses []*dto.ClaudeResponse
			for i := range tt.chunks {
				info.SendResponseCount = i + 1
				responses = append(responses, StreamResponseOpenAI2Claude(&tt.chunks[i], info)...)
			}

			require.Equal(t, tt.toolStarts, countClaudeToolStarts(responses))
			assertClaudeStreamBlockSequence(t, responses)
			assertClaudeStartIndexesContiguous(t, responses)
		})
	}
}

func TestStreamResponseOpenAI2ClaudeSkipsUnstartedToolOffset(t *testing.T) {
	chunks := []dto.ChatCompletionsStreamResponse{
		toolChunk(toolCall(0, "call-metadata", "", "")),
		toolChunk(toolCall(1, "call-1", "lookup", "")),
		toolChunk(toolCallArgs(1, `{"q":"x"}`)),
		finishToolChunk(),
	}
	info := &relaycommon.RelayInfo{
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
	}

	var responses []*dto.ClaudeResponse
	for i := range chunks {
		info.SendResponseCount = i + 1
		responses = append(responses, StreamResponseOpenAI2Claude(&chunks[i], info)...)
	}

	var toolStarts []*dto.ClaudeResponse
	for _, response := range responses {
		if response.Type == "content_block_start" && response.ContentBlock != nil && response.ContentBlock.Type == "tool_use" {
			toolStarts = append(toolStarts, response)
		}
	}
	require.Len(t, toolStarts, 1)
	require.NotNil(t, toolStarts[0].Index)
	require.Equal(t, 0, *toolStarts[0].Index)
	assertClaudeStreamBlockSequence(t, responses)
}

func TestStreamResponseOpenAI2ClaudeToolCallWithoutIDOrIndexAcrossChunks(t *testing.T) {
	chunks := []dto.ChatCompletionsStreamResponse{
		toolChunk(toolCallWithoutIndex("", "lookup")),
		toolChunk(toolCallWithoutIndexWithArgs("", `{"q":"x"}`)),
		finishToolChunk(),
	}
	info := &relaycommon.RelayInfo{
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
	}

	var responses []*dto.ClaudeResponse
	for i := range chunks {
		info.SendResponseCount = i + 1
		responses = append(responses, StreamResponseOpenAI2Claude(&chunks[i], info)...)
	}

	var toolStarts []*dto.ClaudeResponse
	for _, response := range responses {
		if response.Type == "content_block_start" && response.ContentBlock != nil && response.ContentBlock.Type == "tool_use" {
			toolStarts = append(toolStarts, response)
		}
	}
	require.Len(t, toolStarts, 1)
	require.NotNil(t, toolStarts[0].Index)
	require.Equal(t, 0, *toolStarts[0].Index)
	assertClaudeStreamBlockSequence(t, responses)
}

func assertClaudeStreamBlockSequence(t *testing.T, responses []*dto.ClaudeResponse) {
	t.Helper()

	open := make(map[int]bool)
	for _, response := range responses {
		switch response.Type {
		case "content_block_start":
			require.NotNil(t, response.Index)
			index := *response.Index
			require.Falsef(t, open[index], "duplicate content_block_start index=%d", index)
			open[index] = true
		case "content_block_delta":
			require.NotNil(t, response.Index)
			index := *response.Index
			require.Truef(t, open[index], "content_block_delta without start index=%d", index)
		case "content_block_stop":
			require.NotNil(t, response.Index)
			index := *response.Index
			require.Truef(t, open[index], "content_block_stop without start index=%d", index)
			delete(open, index)
		}
	}
	require.Empty(t, open, "all started content blocks must be stopped")
}

func assertClaudeStartIndexesContiguous(t *testing.T, responses []*dto.ClaudeResponse) {
	t.Helper()

	expected := 0
	for _, response := range responses {
		if response.Type != "content_block_start" {
			continue
		}
		require.NotNil(t, response.Index)
		require.Equalf(t, expected, *response.Index, "content block start index has a gap at position %d", expected)
		expected++
	}
}

func countClaudeToolStarts(responses []*dto.ClaudeResponse) int {
	count := 0
	for _, response := range responses {
		if response.Type == "content_block_start" && response.ContentBlock != nil && response.ContentBlock.Type == "tool_use" {
			count++
		}
	}
	return count
}

func toolCall(index int, id, name, arguments string) dto.ToolCallResponse {
	return dto.ToolCallResponse{
		Index: common.GetPointer(index),
		ID:    id,
		Type:  "function",
		Function: dto.FunctionResponse{
			Name:      name,
			Arguments: arguments,
		},
	}
}

func toolCallArgs(index int, arguments string) dto.ToolCallResponse {
	return dto.ToolCallResponse{
		Index:    common.GetPointer(index),
		Type:     "function",
		Function: dto.FunctionResponse{Arguments: arguments},
	}
}

func toolCallWithoutIndex(id, name string) dto.ToolCallResponse {
	return dto.ToolCallResponse{
		ID:       id,
		Type:     "function",
		Function: dto.FunctionResponse{Name: name},
	}
}

func toolCallWithoutIndexWithArgs(id, arguments string) dto.ToolCallResponse {
	return dto.ToolCallResponse{
		ID:       id,
		Type:     "function",
		Function: dto.FunctionResponse{Arguments: arguments},
	}
}

func textChunk(content string) dto.ChatCompletionsStreamResponse {
	return dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
			Content: common.GetPointer(content),
		}}},
	}
}

func thinkingChunk(content string) dto.ChatCompletionsStreamResponse {
	return dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
			ReasoningContent: common.GetPointer(content),
		}}},
	}
}

func toolChunk(toolCalls ...dto.ToolCallResponse) dto.ChatCompletionsStreamResponse {
	return dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
			ToolCalls: toolCalls,
		}}},
	}
}

func finishToolChunk() dto.ChatCompletionsStreamResponse {
	finishReason := "tool_calls"
	return dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			FinishReason: &finishReason,
		}},
		Usage: &dto.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}
}
