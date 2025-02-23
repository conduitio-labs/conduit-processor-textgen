package textgen

import (
	"context"
	"os"
	"testing"

	"github.com/conduitio/conduit-commons/config"
	"github.com/conduitio/conduit-commons/opencdc"
	sdk "github.com/conduitio/conduit-processor-sdk"
	"github.com/matryer/is"
)

func TestProcessor_Process(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	processor := newProcessor(ctx, is,
		"You will receive a json of a list of records. Your task is to output back a json of a list of records with the text of the payloads in uppercase.")

	recs := testRecords()

	processed := processor.Process(ctx, recs)
	is.Equal(len(processed), 3)

	for _, p := range processed {
		switch p := p.(type) {
		case sdk.SingleRecord:
			is.Equal(p.Payload.After, opencdc.RawData("AFT-REC-1"))
		case sdk.FilterRecord:
			is.Fail() // Filter Record should not happen
		case sdk.ErrorRecord:
			is.Equal("", p.Error.Error())
			is.Fail() // empty error record should not happen
		}
	}
}

func newProcessor(ctx context.Context, is *is.I, devMessage string) sdk.Processor {
	processor := NewProcessor()

	apikey := os.Getenv("OPENAI_API_KEY")
	is.True(apikey != "") // OPENAI_API_KEY must be set

	cfg := config.Config{
		ProcessorConfigModel:            "gpt-4o",
		ProcessorConfigApiKey:           apikey,
		ProcessorConfigDeveloperMessage: devMessage,
	}

	is.NoErr(processor.Configure(ctx, cfg))

	return processor
}

func testRecords() []opencdc.Record {
	return []opencdc.Record{
		{
			Operation: opencdc.OperationCreate,
			Key:       opencdc.RawData("key1"),
			Payload: opencdc.Change{
				Before: opencdc.RawData("bef-rec-1"),
				After:  opencdc.RawData("aft-rec-1"),
			},
		},
		{
			Operation: opencdc.OperationUpdate,
			Key:       opencdc.RawData("key2"),
			Payload: opencdc.Change{
				Before: opencdc.RawData("bef-rec-2"),
				After:  opencdc.RawData("aft-rec-2"),
			},
		},
		{
			Operation: opencdc.OperationDelete,
			Key:       opencdc.RawData("key3"),
			Payload: opencdc.Change{
				Before: opencdc.RawData("bef-rec-3"),
				After:  opencdc.RawData("aft-rec-3"),
			},
		},
	}
}
