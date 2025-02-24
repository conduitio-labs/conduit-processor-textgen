package textgen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/conduitio/conduit-commons/config"
	"github.com/conduitio/conduit-commons/opencdc"
	sdk "github.com/conduitio/conduit-processor-sdk"
	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

//go:generate go tool paramgen -output=paramgen_proc.go ProcessorConfig

type Processor struct {
	sdk.UnimplementedProcessor

	config ProcessorConfig

	client *openai.Client
}

type ProcessorConfig struct {
	ApiKey              string            `json:"api_key" validate:"required"`
	DeveloperMessage    string            `json:"developer_message" validate:"required"`
	StrictOutput        bool              `json:"strict_output" default:"false"`
	Model               string            `json:"model" validate:"required"`
	MaxTokens           int               `json:"max_tokens"`
	MaxCompletionTokens int               `json:"max_completion_tokens"`
	Temperature         float32           `json:"temperature"`
	TopP                float32           `json:"top_p"`
	N                   int               `json:"n"`
	Stream              bool              `json:"stream"`
	Stop                []string          `json:"stop"`
	PresencePenalty     float32           `json:"presence_penalty"`
	Seed                *int              `json:"seed"`
	FrequencyPenalty    float32           `json:"frequency_penalty"`
	LogitBias           map[string]int    `json:"logit_bias"`
	LogProbs            bool              `json:"log_probs"`
	TopLogProbs         int               `json:"top_log_probs"`
	User                string            `json:"user"`
	Store               bool              `json:"store"`
	ReasoningEffort     string            `json:"reasoning_effort"`
	Metadata            map[string]string `json:"metadata"`
}

func NewProcessor() sdk.Processor {
	return sdk.ProcessorWithMiddleware(&Processor{}, sdk.DefaultProcessorMiddleware()...)
}

func (p *Processor) Configure(ctx context.Context, cfg config.Config) error {
	err := sdk.ParseConfig(ctx, cfg, &p.config, ProcessorConfig{}.Parameters())
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	if !strings.Contains(p.config.DeveloperMessage, "json") &&
		!strings.Contains(p.config.DeveloperMessage, "JSON") {
		return fmt.Errorf("developer_message must contain 'json' or 'JSON' substrings")
	}

	p.client = openai.NewClient(p.config.ApiKey)

	return nil
}

func (p *Processor) Specification() (sdk.Specification, error) {
	return sdk.Specification{
		Name:        "openai-textgen",
		Summary:     "modify records using openai models",
		Description: "textgen is a conduit processor that will transform a record based on a given prompt",
		Version:     "devel",
		Author:      "Meroxa, Inc.",
		Parameters:  p.config.Parameters(),
	}, nil
}

func (p *Processor) Process(ctx context.Context, recs []opencdc.Record) []sdk.ProcessedRecord {
	res, err := p.createChatCompletion(ctx, recs)
	if err != nil {
		processedRecords := make([]sdk.ProcessedRecord, len(recs))
		for i := range recs {
			processedRecords[i] = sdk.ErrorRecord{Error: err}
		}
		return processedRecords
	}

	var wrappedResponse WantedRecordsResponse
	if err = json.Unmarshal([]byte(res.Choices[0].Message.Content), &wrappedResponse); err != nil {
		processedRecords := make([]sdk.ProcessedRecord, len(recs))
		for i := range recs {
			processedRecords[i] = sdk.ErrorRecord{Error: err}
		}
		return processedRecords
	}

	wantedRecords := wrappedResponse.Records

	processedRecords := make([]sdk.ProcessedRecord, len(recs))
	for i := range recs {
		rec := recs[i]
		wantedRecord := wantedRecords[i]

		if string(rec.Key.Bytes()) == wantedRecord.Key {
			newRec := opencdc.Record{
				Position:  rec.Position,
				Operation: rec.Operation,
				Metadata:  rec.Metadata,
				Key:       rec.Key,
				Payload: opencdc.Change{
					Before: opencdc.RawData([]byte(wantedRecord.Payload.Before)),
					After:  opencdc.RawData([]byte(wantedRecord.Payload.After)),
				},
			}

			processedRecords[i] = sdk.SingleRecord(newRec)
		} else {
			err := fmt.Errorf("key mismatch: %s != %s", string(rec.Key.Bytes()), wantedRecord.Key)
			processedRecords[i] = sdk.ErrorRecord{Error: err}
		}
	}

	return processedRecords
}

func (p *Processor) createChatCompletion(ctx context.Context, records []opencdc.Record) (res openai.ChatCompletionResponse, err error) {
	mappedRecs := make([]WantedRecord, len(records))
	for i := range records {
		mappedRecs[i] = WantedRecord{
			Key: string(records[i].Key.Bytes()),
			Payload: WantedRecordPayload{
				Before: string(records[i].Payload.Before.Bytes()),
				After:  string(records[i].Payload.After.Bytes()),
			},
		}
	}

	bs, err := json.Marshal(mappedRecs)
	if err != nil {
		return res, fmt.Errorf("failed to marshal records: %w", err)
	}

	req := openai.ChatCompletionRequest{
		Model: p.config.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: "developer", Content: p.config.DeveloperMessage},
			{Role: "user", Content: string(bs)},
		},
		MaxTokens:           p.config.MaxTokens,
		MaxCompletionTokens: p.config.MaxCompletionTokens,
		Temperature:         p.config.Temperature,
		TopP:                p.config.TopP,
		N:                   p.config.N,
		Stop:                p.config.Stop,
		PresencePenalty:     p.config.PresencePenalty,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   "openai-textgen",
				Strict: true,
				Schema: &jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"records": {
							Type:  jsonschema.Array,
							Items: &wantedRecordDef,
						},
					},
					Required:             []string{"records"},
					AdditionalProperties: false,
				},
			},
		},
		Seed:             p.config.Seed,
		FrequencyPenalty: p.config.FrequencyPenalty,
		LogitBias:        p.config.LogitBias,
		LogProbs:         p.config.LogProbs,
		TopLogProbs:      p.config.TopLogProbs,
		User:             p.config.User,
		Store:            p.config.Store,
		ReasoningEffort:  p.config.ReasoningEffort,
	}

	if res, err = p.client.CreateChatCompletion(ctx, req); err != nil {
		return res, fmt.Errorf("chat completion failed: %w", err)
	}

	return res, nil
}

type WantedRecord struct {
	Key     string              `json:"key"`
	Payload WantedRecordPayload `json:"payload"`
}

type WantedRecordPayload struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// New type wrapping the records array
type WantedRecordsResponse struct {
	Records []WantedRecord `json:"records"`
}

var wantedRecordDef = jsonschema.Definition{
	Description:          "Represents a record that should be transformed.",
	Type:                 jsonschema.Object,
	AdditionalProperties: false,
	Properties: map[string]jsonschema.Definition{
		"key": {
			Description: "Key represents a value that should identify the entity (e.g. database row).",
			Type:        jsonschema.String, Enum: []string{"string", "object", "null"},
			AdditionalProperties: false,
		},
		"payload": {
			Type:                 jsonschema.Object,
			Description:          "Payload holds the payload change (data before and after the operation occurred).",
			AdditionalProperties: false,
			Properties: map[string]jsonschema.Definition{
				"before": {
					Type: jsonschema.String, Enum: []string{"string", "object", "null"},
					Description: "Before contains the data before the operation occurred. This field is optional and should only be populated for operations OperationUpdate OperationDelete (if the system supports fetching the data before the operation).",
				},
				"after": {
					Type: jsonschema.String, Enum: []string{"string", "object", "null"},
					Description: "After contains the data after the operation occurred. This field should be populated for all operations except OperationDelete.",
				},
			},
			Required: []string{"before", "after"},
		},
	},
	Required: []string{"key", "payload"},
}
