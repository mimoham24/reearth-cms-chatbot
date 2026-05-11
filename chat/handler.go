package chat

import (
	"bytes"
	"cms-chat/client"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	groqURL   = "https://api.groq.com/openai/v1/chat/completions"
	groqModel = "llama-3.3-70b-versatile"
)

const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

type Handler struct {
	cms        *client.Client
	groqKey    string
	httpClient *http.Client
}

func New(cms *client.Client, groqKey string) *Handler {
	return &Handler{
		cms:        cms,
		groqKey:    groqKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// --- Groq API types (OpenAI-compatible) ---

type groqRequest struct {
	Model       string        `json:"model"`
	Messages    []groqMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// --- Main entry point ---

func (h *Handler) Handle(userInput string) (string, error) {
	intent := h.classifyIntent(userInput)
	return h.executeAction(strings.TrimSpace(intent), userInput)
}

// classifyIntent uses Groq when available, falls back to keyword matching.
func (h *Handler) classifyIntent(input string) string {
	if h.groqKey == "" {
		return classifyKeywords(input)
	}

	system := `Classify the user's CMS request. Respond with EXACTLY one of these words — nothing else:
LIST_ITEMS
LIST_MODELS
CREATE_ITEM
SEARCH_ITEMS

LIST_ITEMS:   show/list/get items, data, records, content
LIST_MODELS:  show/list models, schema, structure, fields
CREATE_ITEM:  create/add/insert a new item or record
SEARCH_ITEMS: search/find/filter items by keyword or value`

	result, err := h.askGroq(system, input, 10)
	if err != nil || !isValidIntent(result) {
		return classifyKeywords(input)
	}
	return result
}

func isValidIntent(s string) bool {
	switch strings.TrimSpace(s) {
	case "LIST_ITEMS", "LIST_MODELS", "CREATE_ITEM", "SEARCH_ITEMS":
		return true
	}
	return false
}

// classifyKeywords is a pure-Go fallback intent classifier.
func classifyKeywords(input string) string {
	lower := strings.ToLower(input)
	for _, kw := range []string{"create", "add", "insert", "new item", "new record"} {
		if strings.Contains(lower, kw) {
			return "CREATE_ITEM"
		}
	}
	for _, kw := range []string{"search", "find", "filter", "look for"} {
		if strings.Contains(lower, kw) {
			return "SEARCH_ITEMS"
		}
	}
	for _, kw := range []string{"model", "schema", "structure", "field"} {
		if strings.Contains(lower, kw) {
			return "LIST_MODELS"
		}
	}
	return "LIST_ITEMS"
}

// executeAction dispatches the intent and returns formatted output.
func (h *Handler) executeAction(intent, originalInput string) (string, error) {
	switch intent {
	case "LIST_ITEMS":
		result, err := h.cms.GetItems(1, 10)
		if err != nil {
			return "", err
		}
		return formatItems(result), nil

	case "LIST_MODELS":
		result, err := h.cms.GetModels()
		if err != nil {
			return "", err
		}
		return formatModels(result), nil

	case "SEARCH_ITEMS":
		keyword := extractKeyword(originalInput)
		result, err := h.cms.SearchItems(keyword, 1, 10)
		if err != nil {
			return "", err
		}
		return formatSearchResults(result, keyword), nil

	case "CREATE_ITEM":
		fields, err := h.resolveFields(originalInput)
		if err != nil {
			return "", err
		}
		if len(fields) == 0 {
			return colorYellow + `No fields found. Try: create item title="New Place" description="A location"` + colorReset + "\n", nil
		}
		item, err := h.cms.CreateItem(fields)
		if err != nil {
			return "", err
		}
		return formatCreatedItem(item), nil

	default:
		return "", fmt.Errorf("unknown intent: %q", intent)
	}
}

// resolveFields uses Groq + model schema when available; falls back to regex.
func (h *Handler) resolveFields(input string) ([]client.FieldInput, error) {
	if h.groqKey == "" {
		return parseFields(input), nil
	}

	model, err := h.cms.GetModel()
	if err != nil {
		return parseFields(input), nil // can't fetch schema, use regex
	}

	var schemaDesc strings.Builder
	schemaDesc.WriteString("Available fields:\n")
	for _, f := range model.Schema.Fields {
		schemaDesc.WriteString(fmt.Sprintf("  key=%-20q type=%s\n", f.Key, f.Type))
	}

	system := `Extract CMS field values from the user's request.
` + schemaDesc.String() + `
Respond with ONLY a valid JSON array: [{"key":"field_key","value":"field_value"}]
Only include fields the user mentioned. No explanation, no markdown fences.`

	raw, err := h.askGroq(system, input, 300)
	if err != nil {
		return parseFields(input), nil
	}

	raw = cleanJSON(raw)
	var fields []client.FieldInput
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return parseFields(input), nil // malformed JSON, use regex
	}
	return fields, nil
}

// askGroq sends a chat completion request to Groq's OpenAI-compatible API.
func (h *Handler) askGroq(system, userMsg string, maxTokens int) (string, error) {
	body, err := json.Marshal(groqRequest{
		Model: groqModel,
		Messages: []groqMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: userMsg},
		},
		MaxTokens:   maxTokens,
		Temperature: 0,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", groqURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+h.groqKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Groq request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("Groq API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result groqResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse Groq response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response from Groq")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// cleanJSON strips markdown code fences that LLMs sometimes add.
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// extractKeyword pulls the search term from "search for X" style queries.
func extractKeyword(input string) string {
	lower := strings.ToLower(input)
	for _, trigger := range []string{"for ", "about ", "with "} {
		if idx := strings.Index(lower, trigger); idx != -1 {
			kw := strings.TrimSpace(input[idx+len(trigger):])
			kw = strings.TrimRight(kw, ".,!?")
			if kw != "" {
				return kw
			}
		}
	}
	words := strings.Fields(input)
	if len(words) > 0 {
		return words[len(words)-1]
	}
	return input
}

// parseFields extracts key="value" pairs via regex (fallback for no Groq key).
var (
	quotedFieldRe   = regexp.MustCompile(`(\w+)="([^"]*)"`)
	unquotedFieldRe = regexp.MustCompile(`(\w+)=(\S+)`)
)

func parseFields(input string) []client.FieldInput {
	seen := map[string]bool{}
	var fields []client.FieldInput
	for _, m := range quotedFieldRe.FindAllStringSubmatch(input, -1) {
		seen[m[1]] = true
		fields = append(fields, client.FieldInput{Key: m[1], Value: m[2]})
	}
	for _, m := range unquotedFieldRe.FindAllStringSubmatch(input, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			fields = append(fields, client.FieldInput{Key: m[1], Value: m[2]})
		}
	}
	return fields
}

// --- Output formatters ---

func statusBadge(status string) string {
	switch strings.ToLower(status) {
	case "published", "public":
		return colorGreen + "[" + status + "]" + colorReset
	case "draft":
		return colorYellow + "[" + status + "]" + colorReset
	default:
		if status == "" {
			return ""
		}
		return "[" + status + "]"
	}
}

func fieldLabel(f client.Field) string {
	if f.Key != "" {
		return f.Key
	}
	return f.ID
}

func printItemList(sb *strings.Builder, items []client.Item) {
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("%s%d.%s %s%s%s  %s\n",
			colorBold, i+1, colorReset,
			colorCyan, item.ID, colorReset,
			statusBadge(item.Status)))
		for _, f := range item.Fields {
			sb.WriteString(fmt.Sprintf("     %-20s %v\n", fieldLabel(f)+":", f.Value))
		}
		if len(item.Fields) > 0 {
			sb.WriteString("\n")
		}
	}
}

func formatItems(r *client.ItemsResponse) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%sFetched %d / %d items:%s\n\n",
		colorBold, len(r.Items), r.TotalCount, colorReset))
	if len(r.Items) == 0 {
		sb.WriteString("  (no items)\n")
		return sb.String()
	}
	printItemList(&sb, r.Items)
	return sb.String()
}

func formatSearchResults(r *client.ItemsResponse, keyword string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%sSearch %q — %d / %d items:%s\n\n",
		colorBold, keyword, len(r.Items), r.TotalCount, colorReset))
	if len(r.Items) == 0 {
		sb.WriteString("  (no matches)\n")
		return sb.String()
	}
	printItemList(&sb, r.Items)
	return sb.String()
}

func formatModels(r *client.ModelsResponse) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s%d model(s):%s\n\n", colorBold, len(r.Models), colorReset))
	for _, m := range r.Models {
		sb.WriteString(fmt.Sprintf("  %s%s%s  key: %s\n", colorCyan, m.Name, colorReset, m.Key))
		if m.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", m.Description))
		}
		for _, f := range m.Schema.Fields {
			sb.WriteString(fmt.Sprintf("    %-20s %s\n", f.Key+":", f.Type))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatCreatedItem(item *client.VersionedItem) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s%sItem created!%s\n\n", colorGreen, colorBold, colorReset))
	sb.WriteString(fmt.Sprintf("  ID:       %s\n", item.ID))
	if item.Version != "" {
		sb.WriteString(fmt.Sprintf("  Version:  %s\n", item.Version))
	}
	if len(item.Refs) > 0 {
		sb.WriteString(fmt.Sprintf("  Status:   %s\n", statusBadge(item.Refs[0])))
	}
	if len(item.Fields) > 0 {
		sb.WriteString("\n  Fields:\n")
		for _, f := range item.Fields {
			sb.WriteString(fmt.Sprintf("    %-20s %v\n", fieldLabel(f)+":", f.Value))
		}
	}
	return sb.String()
}
