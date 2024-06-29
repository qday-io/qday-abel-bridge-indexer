package cmd

import (
	"context"
	"testing"

	"github.com/b2network/b2-indexer/internal/handler"
	"github.com/b2network/b2-indexer/internal/types"
)

func Test_startCmd(t *testing.T) {
	cmd := buildIndexCmd()
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.ServerContextKey, handler.NewDefaultContext())
	cmd.SetContext(ctx)
	err := cmd.Execute()
	if err != nil {
		panic(err)
	}
}
