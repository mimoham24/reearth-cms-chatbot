package chat

import (
	"bytes"
	"cms-chat/client"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
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
SEARCH_ITEMS

LIST_ITEMS:   show/list/get items, data, records, content
LIST_MODELS:  show/list models, schema, structure, fields
SEARCH_ITEMS: search/find/filter items by keyword or value`

	result, err := h.askGroq(system, input, 10)
	if err != nil || !isValidIntent(result) {
		return classifyKeywords(input)
	}
	return result
}

func isValidIntent(s string) bool {
	switch strings.TrimSpace(s) {
	case "LIST_ITEMS", "LIST_MODELS", "SEARCH_ITEMS":
		return true
	}
	return false
}

// classifyKeywords is a pure-Go fallback intent classifier.
func classifyKeywords(input string) string {
	lower := strings.ToLower(input)
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
		filters := h.extractFilters(originalInput)
		all, err := h.cms.GetItems(1, 100)
		if err != nil {
			return "", err
		}
		matched := filterItemsStructured(all.Items, filters)
		return formatSearchResults(matched, all.TotalCount, filters.label()), nil

	default:
		return "", fmt.Errorf("unknown intent: %q", intent)
	}
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

// --- Structured search ---

type searchFilters struct {
	Category   string   `json:"category"`
	City       string   `json:"city"`
	Keywords   []string `json:"keywords"`
	YearBefore *int     `json:"year_before"`
	YearAfter  *int     `json:"year_after"`
}

func (f searchFilters) label() string {
	var parts []string
	if f.Category != "" {
		parts = append(parts, f.Category)
	}
	if f.City != "" {
		parts = append(parts, f.City)
	}
	parts = append(parts, f.Keywords...)
	if f.YearBefore != nil {
		parts = append(parts, fmt.Sprintf("before %d AD", *f.YearBefore))
	}
	if f.YearAfter != nil {
		parts = append(parts, fmt.Sprintf("after %d AD", *f.YearAfter))
	}
	return strings.Join(parts, " + ")
}

var yearRe = regexp.MustCompile(`\b(\d{3,4})\b`)

// extractFilters uses Groq to parse structured filters; falls back to keywords.
func (h *Handler) extractFilters(input string) searchFilters {
	if h.groqKey != "" {
		system := `Extract search filters from the user's query. Respond with ONLY valid JSON — no explanation, no markdown:
{"category": "shrine|temple|castle|garden|museum|historic or empty string", "city": "city name with correct casing or empty string", "keywords": ["other", "significant", "words"], "year_before": null or integer, "year_after": null or integer}
Only populate category if the user clearly means a place type. Only populate city if a city is mentioned.
For year constraints like "before 1000 AD" set year_before=1000. For "after 1500" set year_after=1500.`

		raw, err := h.askGroq(system, input, 120)
		if err == nil {
			raw = cleanJSON(raw)
			var f searchFilters
			if json.Unmarshal([]byte(raw), &f) == nil {
				return f
			}
		}
	}
	// fallback: treat everything as keywords
	return searchFilters{Keywords: strings.Fields(extractKeyword(input))}
}

func filterItemsStructured(items []client.Item, f searchFilters) []client.Item {
	var results []client.Item
	for _, item := range items {
		if matchesFilters(item, f) {
			results = append(results, item)
		}
	}
	return results
}

func matchesFilters(item client.Item, f searchFilters) bool {
	fields := make(map[string]string)
	for _, field := range item.Fields {
		fields[field.Key] = strings.ToLower(fmt.Sprintf("%v", field.Value))
	}

	if f.Category != "" && !strings.Contains(fields["category"], strings.ToLower(f.Category)) {
		return false
	}
	if f.City != "" && !strings.Contains(fields["city"], strings.ToLower(f.City)) {
		return false
	}
	if len(f.Keywords) > 0 {
		var allText strings.Builder
		for _, v := range fields {
			allText.WriteString(v)
			allText.WriteString(" ")
		}
		text := allText.String()
		for _, kw := range f.Keywords {
			if !strings.Contains(text, strings.ToLower(kw)) {
				return false
			}
		}
	}
	if f.YearBefore != nil || f.YearAfter != nil {
		var textOnly strings.Builder
		for _, field := range item.Fields {
			v := fmt.Sprintf("%v", field.Value)
			// skip geometry / array values — they contain coordinate numbers
			if !strings.ContainsAny(v, "{[") {
				textOnly.WriteString(strings.ToLower(v))
				textOnly.WriteString(" ")
			}
		}
		years := extractYears(textOnly.String())
		if !yearMatches(years, f.YearBefore, f.YearAfter) {
			return false
		}
	}
	return true
}

func extractYears(text string) []int {
	var years []int
	for _, m := range yearRe.FindAllString(text, -1) {
		if y, err := strconv.Atoi(m); err == nil && y > 100 && y <= 2100 {
			years = append(years, y)
		}
	}
	return years
}

func yearMatches(years []int, before, after *int) bool {
	if len(years) == 0 {
		return false
	}
	for _, y := range years {
		ok := true
		if before != nil && y >= *before {
			ok = false
		}
		if after != nil && y <= *after {
			ok = false
		}
		if ok {
			return true
		}
	}
	return false
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// filterItems returns items whose field values match all keywords in the query.
func filterItems(items []client.Item, query string) []client.Item {
	var keywords []string
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if len(w) > 2 && !stopWord[w] {
			keywords = append(keywords, w)
		}
	}
	if len(keywords) == 0 {
		return items
	}

	var results []client.Item
	for _, item := range items {
		if itemMatches(item, keywords) {
			results = append(results, item)
		}
	}
	return results
}

var stopWord = map[string]bool{
	"for": true, "the": true, "and": true, "or": true,
	"in": true, "at": true, "of": true, "to": true,
	"from": true, "a": true, "an": true,
}

func itemMatches(item client.Item, keywords []string) bool {
	var sb strings.Builder
	for _, f := range item.Fields {
		sb.WriteString(strings.ToLower(fmt.Sprintf("%v", f.Value)))
		sb.WriteString(" ")
	}
	text := sb.String()
	for _, kw := range keywords {
		if !strings.Contains(text, kw) {
			return false
		}
	}
	return true
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

func formatSearchResults(items []client.Item, total int, keyword string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%sSearch %q — %d / %d items:%s\n\n",
		colorBold, keyword, len(items), total, colorReset))
	if len(items) == 0 {
		sb.WriteString("  (no matches)\n")
		return sb.String()
	}
	printItemList(&sb, items)
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

