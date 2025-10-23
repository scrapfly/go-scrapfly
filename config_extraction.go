package scrapfly

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// CompressionFormat for document compression.
type CompressionFormat string

const (
	GZIP    CompressionFormat = "gzip"
	ZSTD    CompressionFormat = "zstd"
	DEFLATE CompressionFormat = "deflate"
)

// ExtractionConfig holds parameters for an extraction request.
type ExtractionConfig struct {
	Body                        []byte
	ContentType                 string
	URL                         string
	Charset                     string
	ExtractionTemplate          string
	ExtractionEphemeralTemplate map[string]interface{}
	ExtractionPrompt            string
	ExtractionModel             string
	IsDocumentCompressed        bool
	DocumentCompressionFormat   CompressionFormat
	Webhook                     string
}

// toAPIParams converts the ExtractionConfig into URL parameters.
func (c *ExtractionConfig) toAPIParams() (url.Values, error) {
	params := url.Values{}

	if len(c.Body) == 0 {
		return nil, fmt.Errorf("%w: Body is required", ErrExtractionConfig)
	}
	if c.ContentType == "" {
		return nil, fmt.Errorf("%w: ContentType is required", ErrExtractionConfig)
	}

	params.Set("content_type", c.ContentType)

	if c.URL != "" {
		params.Set("url", c.URL)
	}
	if c.Charset != "" {
		params.Set("charset", c.Charset)
	}

	if c.ExtractionTemplate != "" && c.ExtractionEphemeralTemplate != nil {
		return nil, fmt.Errorf("%w: cannot use both extraction_template and extraction_ephemeral_template", ErrExtractionConfig)
	}
	if c.ExtractionTemplate != "" {
		params.Set("extraction_template", c.ExtractionTemplate)
	}
	if c.ExtractionEphemeralTemplate != nil {
		templateJSON, err := json.Marshal(c.ExtractionEphemeralTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal extraction_ephemeral_template: %w", err)
		}
		params.Set("extraction_template", "ephemeral:"+urlSafeB64Encode(string(templateJSON)))
	}
	if c.ExtractionPrompt != "" {
		params.Set("extraction_prompt", c.ExtractionPrompt)
	}
	if c.ExtractionModel != "" {
		params.Set("extraction_model", c.ExtractionModel)
	}

	if c.Webhook != "" {
		params.Set("webhook_name", c.Webhook)
	}

	return params, nil
}