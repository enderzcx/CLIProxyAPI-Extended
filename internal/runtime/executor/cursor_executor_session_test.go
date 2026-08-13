package executor

import (
	"testing"

	"github.com/tidwall/gjson"
)

func responseMessages(raw string) []gjson.Result {
	return gjson.Get(raw, "messages").Array()
}

func TestDeriveConversationIDUsesExplicitSessionID(t *testing.T) {
	first := deriveConversationId("key", "session-1", "grok-4.5", "system-a", nil)
	second := deriveConversationId("key", "session-1", "grok-4.6", "system-b", responseMessages(`{"messages":[{"role":"user","content":"different"}]}`))
	if first != second {
		t.Fatalf("explicit session ID must remain authoritative: %q != %q", first, second)
	}
}

func TestDeriveConversationIDSeparatesResponsesConversationsWithoutMetadata(t *testing.T) {
	mainMessages := responseMessages(`{"messages":[{"role":"user","content":"Use the shell tool to run pwd"}]}`)
	titleMessages := responseMessages(`{"messages":[{"role":"user","content":"Generate a session title"}]}`)

	mainID := deriveConversationId("shared-key", "", "grok-4.6", "You are a helpful assistant.", mainMessages)
	titleID := deriveConversationId("shared-key", "", "grok-4.5", "You are a helpful assistant.", titleMessages)
	if mainID == titleID {
		t.Fatalf("independent Responses conversations collapsed onto %q", mainID)
	}

	otherPromptID := deriveConversationId("shared-key", "", "grok-4.6", "You are a helpful assistant.", titleMessages)
	if mainID == otherPromptID {
		t.Fatalf("same-model conversations with different first prompts collapsed onto %q", mainID)
	}
}

func TestDeriveConversationIDStaysStableAcrossToolResultTurn(t *testing.T) {
	firstTurn := responseMessages(`{"messages":[{"role":"user","content":"Use the shell tool to run pwd"}]}`)
	secondTurn := responseMessages(`{"messages":[{"role":"user","content":"Use the shell tool to run pwd"},{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"run_terminal_command","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call-1","content":"/workspace"}]}`)

	firstID := deriveConversationId("shared-key", "", "grok-4.6", "You are a helpful assistant.", firstTurn)
	secondID := deriveConversationId("shared-key", "", "grok-4.6", "You are a helpful assistant.", secondTurn)
	if firstID != secondID {
		t.Fatalf("tool-result turn changed conversation ID: %q != %q", firstID, secondID)
	}
}

func TestCursorToolCallIDsMatchCompositeResponsesID(t *testing.T) {
	if !cursorToolCallIDsMatch("call-real\nfc_synthetic_0", "call-real") {
		t.Fatal("composite Responses ID did not match the pending Cursor call ID")
	}
	if cursorToolCallIDsMatch("fc_synthetic_0", "call-real") {
		t.Fatal("unrelated synthetic item ID matched the pending Cursor call ID")
	}
}
