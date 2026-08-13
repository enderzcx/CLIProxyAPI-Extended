package executor

import (
	"testing"

	cursorproto "github.com/router-for-me/CLIProxyAPI/v6/internal/auth/cursor/proto"
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
	if !cursorToolCallIDsMatch("fc_synthetic_0", "call-real\nfc_synthetic_0") {
		t.Fatal("synthetic Responses item ID did not match the composite pending Cursor ID")
	}
	if cursorToolCallIDsMatch("fc_synthetic_0", "call-real") {
		t.Fatal("unrelated synthetic item ID matched the pending Cursor call ID")
	}
}

func TestParseOpenAIRequestAddsToolCommitDirective(t *testing.T) {
	payload := []byte(`{"model":"grok-4.6","messages":[{"role":"user","content":"Run pwd with the shell tool"}],"tools":[{"type":"function","function":{"name":"run_terminal_command","parameters":{"type":"object"}}}]}`)
	parsed := parseOpenAIRequest(payload)

	if got := parsed.UserText; got != cursorToolCommitDirective+"\n\nRun pwd with the shell tool" {
		t.Fatalf("user text = %q", got)
	}
}

func TestParseOpenAIRequestDoesNotAddToolCommitDirectiveWithoutTools(t *testing.T) {
	payload := []byte(`{"model":"grok-4.6","messages":[{"role":"user","content":"Say hello"}]}`)
	parsed := parseOpenAIRequest(payload)

	if got := parsed.UserText; got != "Say hello" {
		t.Fatalf("user text = %q", got)
	}
}

func TestCursorToolCommitDirectiveIsNotRepeated(t *testing.T) {
	parsed := &parsedOpenAIRequest{
		UserText: cursorToolCommitDirective + "\n\nRun pwd",
		Tools:    responseMessages(`{"messages":[{}]}`),
	}
	applyCursorToolCommitDirective(parsed)

	if got := parsed.UserText; got != cursorToolCommitDirective+"\n\nRun pwd" {
		t.Fatalf("user text = %q", got)
	}
}

func TestResolveCursorDeclaredToolNamePrefersExactShellAlias(t *testing.T) {
	tools := []cursorproto.McpToolDef{
		{Name: "search_tool"},
		{Name: "run_terminal_command"},
		{Name: "use_tool"},
	}

	if got := resolveCursorDeclaredToolName(tools, "run_terminal_command", "shell"); got != "run_terminal_command" {
		t.Fatalf("tool name = %q", got)
	}
}

func TestResolveCursorDeclaredToolNameDoesNotGuessUnrelatedTool(t *testing.T) {
	tools := []cursorproto.McpToolDef{{Name: "search_tool"}, {Name: "use_tool"}}
	if got := resolveCursorDeclaredToolName(tools, "run_terminal_command", "shell"); got != "" {
		t.Fatalf("tool name = %q, want empty", got)
	}
}

func TestEncodeExecShellSuccessProducesClientMessage(t *testing.T) {
	encoded := cursorproto.EncodeExecShellSuccess(7, "exec-1", "pwd", "/workspace", "/workspace\n")
	if len(encoded) == 0 {
		t.Fatal("shell success encoded to an empty message")
	}
}

func TestEncodeExecShellStreamSuccessProducesStartOutputAndExit(t *testing.T) {
	encoded := cursorproto.EncodeExecShellStreamSuccess(7, "exec-1", "/workspace", "/workspace\n")
	if len(encoded) != 3 {
		t.Fatalf("shell stream encoded %d messages, want start, stdout, exit", len(encoded))
	}
	for i, message := range encoded {
		if len(message) == 0 {
			t.Fatalf("shell stream message %d is empty", i)
		}
	}
}

func TestCursorBuiltinShellToolUsesExternalMcpAlias(t *testing.T) {
	upstream := cursorUpstreamToolName("run_terminal_command")
	if upstream == "run_terminal_command" {
		t.Fatal("builtin-colliding shell tool was not aliased")
	}
	if got := cursorClientToolName(upstream); got != "run_terminal_command" {
		t.Fatalf("restored tool name = %q", got)
	}
	if got := cursorUpstreamToolName("search_tool"); got != "search_tool" {
		t.Fatalf("unrelated tool name changed to %q", got)
	}
	tools := []cursorproto.McpToolDef{{Name: upstream}}
	if got := resolveCursorDeclaredToolName(tools, "run_terminal_command", "shell"); got != "" {
		t.Fatalf("external MCP alias was incorrectly bridged back to builtin shell as %q", got)
	}
}
