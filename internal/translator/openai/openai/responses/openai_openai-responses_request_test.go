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
