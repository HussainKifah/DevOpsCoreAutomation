package syslog

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/elastic/go-elasticsearch/v9"
)

// Client wraps the official ES client for syslog-style searches.
type Client struct {
	es      *elasticsearch.Client
	indexPat string
}

// NewClient returns nil, nil if ELASTICSEARCH_URL is not set.
func NewClient(cfg *config.Config) (*Client, error) {
	url := strings.TrimSpace(cfg.ElasticsearchURL)
	if url == "" {
		return nil, nil
	}
	if !strings.Contains(url, "://") {
		url = "http://" + url
	}
	escfg := elasticsearch.Config{
		Addresses: []string{url},
	}
	if cfg.ElasticsearchUser != "" {
		escfg.Username = cfg.ElasticsearchUser
		escfg.Password = cfg.ElasticsearchPassword
	}
	if cfg.ElasticsearchSkipTLSVerify {
		escfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}
	es, err := elasticsearch.NewClient(escfg)
	if err != nil {
		return nil, err
	}
	pat := strings.TrimSpace(cfg.ElasticsearchIndexPattern)
	if pat == "" {
		pat = "logstash-*"
	}
	return &Client{es: es, indexPat: pat}, nil
}

// Hit is one Elasticsearch document from a search.
type Hit struct {
	Index   string
	ID      string
	Source  Source
	RawJSON json.RawMessage
}

// Source maps common logstash fields.
type Source struct {
	Timestamp string `json:"@timestamp"`
	Host      string `json:"host"`
	Message   string `json:"message"`
}

type esSearchResponse struct {
	Hits struct {
		Hits []struct {
			Index  string          `json:"_index"`
			ID     string          `json:"_id"`
			Source json.RawMessage `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

// SearchLastWindow runs a bool query: @timestamp in [from, to] AND match_phrase on message.
// Each UI filter runs as its own search; hits from filter1 ∪ filter2 are merged in the poller (OR across filters, deduped by doc id).
func (c *Client) SearchLastWindow(ctx context.Context, messageQuery string, from, to time.Time) ([]Hit, error) {
	body := map[string]interface{}{
		"size": 500,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []interface{}{
					map[string]interface{}{
						"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{
								"gte": from.UTC().Format(time.RFC3339Nano),
								"lte": to.UTC().Format(time.RFC3339Nano),
							},
						},
					},
					map[string]interface{}{
						// match_phrase: exact token sequence in message (no "operator" — that exists only on "match")
						"match_phrase": map[string]interface{}{
							"message": map[string]interface{}{
								"query": messageQuery,
							},
						},
					},
				},
			},
		},
		"sort": []interface{}{
			map[string]interface{}{"@timestamp": "desc"},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(c.indexPat),
		c.es.Search.WithBody(strings.NewReader(string(raw))),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch: %s", string(b))
	}
	var parsed esSearchResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, err
	}
	out := make([]Hit, 0, len(parsed.Hits.Hits))
	for _, h := range parsed.Hits.Hits {
		var src Source
		_ = json.Unmarshal(h.Source, &src)
		out = append(out, Hit{
			Index:   h.Index,
			ID:      h.ID,
			Source:  src,
			RawJSON: h.Source,
		})
	}
	return out, nil
}

// DeviceNameFromMessage returns text before the first ':' (e.g. "KUT-WiFi" from "KUT-WiFi: 1343...").
func DeviceNameFromMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	i := strings.Index(msg, ":")
	if i <= 0 {
		return ""
	}
	s := strings.TrimSpace(msg[:i])
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
