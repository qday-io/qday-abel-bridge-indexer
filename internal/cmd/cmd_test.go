package cmd

import (
	"context"
	"testing"

	"github.com/b2network/b2-indexer/internal/server"
	"github.com/b2network/b2-indexer/internal/types"
)

func Test_startCmd(t *testing.T) {
	cmd := startCmd()
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.ServerContextKey, server.NewDefaultContext())
	cmd.SetContext(ctx)
	err := cmd.Execute()
	if err != nil {
		panic(err)
	}
}

func Test_startHTTPServer(t *testing.T) {
	startHTTPServer()
}
