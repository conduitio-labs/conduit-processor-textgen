package textgen

import (
	"context"
	"fmt"

	"github.com/conduitio/conduit-commons/config"
	"github.com/conduitio/conduit-commons/opencdc"
	sdk "github.com/conduitio/conduit-processor-sdk"
	"github.com/sashabaranov/go-openai"
)

//go:generate go tool paramgen -output=paramgen_proc.go ProcessorConfig

type Processor struct {
	sdk.UnimplementedProcessor

	config            ProcessorConfig
	client            *openai.Client
	referenceResolver sdk.ReferenceResolver
}

type ProcessorConfig struct {
	Field               string            `json:"field" default:".Payload.After"`
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

	p.referenceResolver, err = sdk.NewReferenceResolver(p.config.Field)
	if err != nil {
		return fmt.Errorf("failed to create reference resolver: %w", err)
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
	processedRecords := make([]sdk.ProcessedRecord, len(recs))
	for i, rec := range recs {
		processed, err := p.processRecord(ctx, rec)
		if err != nil {
			processedRecords[i] = sdk.ErrorRecord{Error: err}
			continue
		}

		processedRecords[i] = sdk.SingleRecord(processed)
	}

	return processedRecords
}

func (p *Processor) processRecord(ctx context.Context, rec opencdc.Record) (opencdc.Record, error) {
	logger := sdk.Logger(ctx)

	ref, err := p.referenceResolver.Resolve(&rec)
	if err != nil {
		return rec, fmt.Errorf("failed to resolve reference: %w", err)
	}

	val := ref.Get()

	var payload string
	switch v := val.(type) {
	case opencdc.Position:
		payload = string(v)

		res, err := p.createChatCompletion(ctx, payload)
		if err != nil {
			return rec, fmt.Errorf("failed to create chat completion: %w", err)
		}

		logger.Trace().Msgf("processed record position %s", res)

		if err := ref.Set(opencdc.Position(res)); err != nil {
			return rec, fmt.Errorf("failed to set position: %w", err)
		}
	case opencdc.Data:
		payload = string(v.Bytes())

		res, err := p.createChatCompletion(ctx, payload)
		if err != nil {
			return rec, fmt.Errorf("failed to create chat completion: %w", err)
		}

		logger.Trace().Msgf("processed record data %s", res)

		var data opencdc.Data = opencdc.RawData(res)

		if err := ref.Set(data); err != nil {
			return rec, fmt.Errorf("failed to set data: %w", err)
		}

	case string:
		payload = v

		res, err := p.createChatCompletion(ctx, payload)
		if err != nil {
			return rec, fmt.Errorf("failed to create chat completion: %w", err)
		}

		logger.Trace().Msgf("processed record string %s", res)

		if err := ref.Set(res); err != nil {
			return rec, fmt.Errorf("failed to set data: %w", err)
		}
	default:
		return rec, fmt.Errorf("unsupported type %T", v)
	}

	return rec, nil
}

func (p *Processor) createChatCompletion(ctx context.Context, payload string) (string, error) {
	req := openai.ChatCompletionRequest{
		Model: p.config.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: "developer", Content: p.config.DeveloperMessage},
			{Role: "user", Content: payload},
		},
		MaxTokens:           p.config.MaxTokens,
		MaxCompletionTokens: p.config.MaxCompletionTokens,
		Temperature:         p.config.Temperature,
		TopP:                p.config.TopP,
		N:                   p.config.N,
		Stop:                p.config.Stop,
		PresencePenalty:     p.config.PresencePenalty,
		Seed:                p.config.Seed,
		FrequencyPenalty:    p.config.FrequencyPenalty,
		LogitBias:           p.config.LogitBias,
		LogProbs:            p.config.LogProbs,
		TopLogProbs:         p.config.TopLogProbs,
		User:                p.config.User,
		Store:               p.config.Store,
		ReasoningEffort:     p.config.ReasoningEffort,
	}

	res, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("chat completion failed: %w", err)
	}

	return res.Choices[0].Message.Content, nil
}
