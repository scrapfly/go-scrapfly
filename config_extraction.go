package scrapfly

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// CompressionFormat specifies the compression format for document body.
type CompressionFormat string

// Available compression formats for document body compression.
const (
	// GZIP uses gzip compression (widely supported, good compression ratio).
	GZIP CompressionFormat = "gzip"
	// ZSTD uses Zstandard compression (better compression and speed than gzip).
	ZSTD CompressionFormat = "zstd"
	// DEFLATE uses DEFLATE compression (older format, similar to gzip).
	DEFLATE CompressionFormat = "deflate"
)

// ExtractionConfig configures an AI-powered data extraction request to the Scrapfly API.
//
// This struct contains all available options for extracting structured data from
// HTML or other document formats using AI models or predefined templates.
//
// Example with template:
//
//	config := &scrapfly.ExtractionConfig{
//	    Body:               []byte("<html>...</html>"),
//	    ContentType:        "text/html",
//	    ExtractionTemplate: "product",
//	}
//
// Example with AI prompt:
//
//	config := &scrapfly.ExtractionConfig{
//	    Body:             []byte("<html>...</html>"),
//	    ContentType:      "text/html",
//	    ExtractionPrompt: "Extract product name, price, and description",
//	}
type ExtractionConfig struct {
	// Body is the document content to extract data from (required).
	Body []byte `required:"true"`
	// ContentType specifies the document content type, e.g., "text/html" (required).
	ContentType string `required:"true"`
	// URL is the original URL of the document (optional, helps with context).
	URL string
	// Charset specifies the character encoding of the document.
	Charset string
	// ExtractionTemplate is the name of a saved extraction template.
	ExtractionTemplate string `exclusive:"extraction"`
	// ExtractionEphemeralTemplate is an inline extraction template definition.
	ExtractionEphemeralTemplate map[string]interface{} `exclusive:"extraction"`
	// ExtractionPrompt is an AI prompt describing what data to extract.
	ExtractionPrompt string `exclusive:"extraction"`
	// ExtractionModel specifies which AI model to use for extraction.
	ExtractionModel ExtractionModel `exclusive:"extraction" validate:"enum"`
	// IsDocumentCompressed indicates if the Body is compressed.
	IsDocumentCompressed bool
	// DocumentCompressionFormat specifies the compression format if IsDocumentCompressed is true.
	DocumentCompressionFormat CompressionFormat
	// Webhook is the name of a webhook to call after extraction completes.
	Webhook string
}

// toAPIParams converts the ExtractionConfig into URL parameters for the Scrapfly API.
// This is an internal method used by the Client to prepare API requests.
func (c *ExtractionConfig) toAPIParams() (url.Values, error) {

	// validate exclusive fields, see struct tags
	if err := ValidateExclusiveFields(c); err != nil {
		return nil, err
	}
	// validate required fields, see struct tags
	if err := ValidateRequiredFields(c); err != nil {
		return nil, err
	}
	// validate enums, see struct tags
	if err := ValidateEnums(c); err != nil {
		return nil, err
	}

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
		params.Set("extraction_model", string(c.ExtractionModel))
	}

	if c.Webhook != "" {
		params.Set("webhook_name", c.Webhook)
	}

	return params, nil
}
