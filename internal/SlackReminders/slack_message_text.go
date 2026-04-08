package slackreminders

import (
	"encoding/json"
	"strings"
)

// ExtractMessageTextFromSlackEvent builds searchable text from a Slack message event.
// Workflows and Block Kit often leave top-level "text" empty; attachments/blocks hold the body.
func ExtractMessageTextFromSlackEvent(raw json.RawMessage) string {
	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return ""
	}
	return strings.TrimSpace(collectSlackMessageText(root))
}

func collectSlackMessageText(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	var parts []string
	if t, ok := m["text"].(string); ok {
		if s := strings.TrimSpace(t); s != "" {
			parts = append(parts, s)
		}
	}
	if atts, ok := m["attachments"].([]interface{}); ok {
		for _, a := range atts {
			am, _ := a.(map[string]interface{})
			for _, key := range []string{"pretext", "text", "fallback"} {
				if s, ok := am[key].(string); ok {
					if ts := strings.TrimSpace(s); ts != "" {
						parts = append(parts, ts)
					}
				}
			}
		}
	}
	if blocks, ok := m["blocks"].([]interface{}); ok {
		parts = append(parts, collectFromBlocks(blocks)...)
	}
	return strings.Join(parts, "\n")
}

func collectFromBlocks(blocks []interface{}) []string {
	var out []string
	for _, b := range blocks {
		out = append(out, collectBlockNode(b)...)
	}
	return out
}

func collectBlockNode(v interface{}) []string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	var out []string
	typ, _ := m["type"].(string)
	switch typ {
	case "section", "header":
		if tx, ok := m["text"].(map[string]interface{}); ok {
			if s, ok := tx["text"].(string); ok {
				if ts := strings.TrimSpace(s); ts != "" {
					out = append(out, ts)
				}
			}
		}
	case "context":
		if els, ok := m["elements"].([]interface{}); ok {
			for _, e := range els {
				out = append(out, collectBlockNode(e)...)
			}
		}
	case "rich_text":
		if els, ok := m["elements"].([]interface{}); ok {
			for _, e := range els {
				out = append(out, collectBlockNode(e)...)
			}
		}
	case "rich_text_section":
		if els, ok := m["elements"].([]interface{}); ok {
			for _, e := range els {
				out = append(out, collectRichTextElement(e)...)
			}
		}
	}
	for _, key := range []string{"elements", "fields"} {
		if arr, ok := m[key].([]interface{}); ok {
			for _, e := range arr {
				out = append(out, collectBlockNode(e)...)
			}
		}
	}
	return out
}

func collectRichTextElement(v interface{}) []string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	var out []string
	if s, ok := m["text"].(string); ok {
		if ts := strings.TrimSpace(s); ts != "" {
			out = append(out, ts)
		}
	}
	if els, ok := m["elements"].([]interface{}); ok {
		for _, e := range els {
			out = append(out, collectRichTextElement(e)...)
		}
	}
	return out
}
