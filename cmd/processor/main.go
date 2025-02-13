//go:build wasm

package main

import (
	sdk "github.com/conduitio/conduit-processor-sdk"
	textgen "github.com/conduitio/conduit-processor-template"
)

func main() {
	sdk.Run(textgen.NewProcessor())
}
