package responses

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertResponsesRequestNormalizesCompositeCursorToolCallID(t *testing.T) {
	raw := []byte(`{
		"model":"grok-4.6",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"run pwd"}]},
			{"type":"function_call","call_id":"call-real\nfc_synthetic_0","name":"get_pwd","arguments":"{}"},
			{"type":"function_call_output","call_id":"call-real\nfc_synthetic_0","output":"/workspace"}
		]
	}`)

	converted := ConvertOpenAIResponsesRequestToOpenAIChatCompletions("cursor-grok-4.6-high", raw, true)
	assistantID := gjson.GetBytes(converted, "messages.1.tool_calls.0.id").String()
	resultID := gjson.GetBytes(converted, "messages.2.tool_call_id").String()
	if assistantID != "call-real" {
		t.Fatalf("assistant tool call id = %q, want call-real", assistantID)
	}
	if resultID != "call-real" {
		t.Fatalf("tool result id = %q, want call-real", resultID)
	}
}

func TestConvertResponsesRequestPreservesCustomToolAndOutput(t *testing.T) {
	raw := []byte(`{
		"model":"grok-4.6",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"patch file"}]},
			{"type":"custom_tool_call","call_id":"call-patch\nctc_synthetic_0","name":"apply_patch","input":"*** Begin Patch"},
			{"type":"custom_tool_call_output","call_id":"call-patch\nctc_synthetic_0","output":"Done!"}
		],
		"tools":[{"type":"custom","name":"apply_patch","description":"Apply patch","format":{"type":"text"}}]
	}`)

	converted := ConvertOpenAIResponsesRequestToOpenAIChatCompletions("cursor-grok-4.6-xhigh", raw, true)
	if got := gjson.GetBytes(converted, "tools.0.type").String(); got != "custom" {
		t.Fatalf("custom tool type = %q", got)
	}
	if got := gjson.GetBytes(converted, "messages.1.tool_calls.0.function.name").String(); got != "apply_patch" {
		t.Fatalf("custom tool name = %q", got)
	}
	if got := gjson.Get(gjson.GetBytes(converted, "messages.1.tool_calls.0.function.arguments").String(), "input").String(); got != "*** Begin Patch" {
		t.Fatalf("custom tool input = %q", got)
	}
	if got := gjson.GetBytes(converted, "messages.2.tool_call_id").String(); got != "call-patch" {
		t.Fatalf("custom result id = %q", got)
	}
}
