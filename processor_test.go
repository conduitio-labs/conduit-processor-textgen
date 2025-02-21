package textgen

import (
	"context"
	"os"
	"testing"

	"github.com/conduitio/conduit-commons/config"
	sdk "github.com/conduitio/conduit-processor-sdk"
	"github.com/matryer/is"
)

func TestProcessor_Process(t *testing.T) {

}

func newProcessor(ctx context.Context, is *is.I, devMessage string) *Processor {
	processor := &Processor{}

	apikey := os.Getenv("OPENAI_API_KEY")
	is.True(apikey != "") // OPENAI_API_KEY must be set

	cfg := config.Config{
		"openai_api_key":    apikey,
		"developer_message": devMessage,
	}
	err := sdk.ParseConfig(ctx, cfg, &processor.config, ProcessorConfig{}.Parameters())
	is.NoErr(err)

	return processor
}
