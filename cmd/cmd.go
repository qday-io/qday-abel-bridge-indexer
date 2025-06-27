package cmd

import (
	"context"
	"os"

	"github.com/qday-io/qday-abel-bridge-indexer/internal/handler"
	"github.com/qday-io/qday-abel-bridge-indexer/internal/model"
	"github.com/qday-io/qday-abel-bridge-indexer/pkg/log"
	"github.com/spf13/cobra"
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "abe-indexer",
		Short: "index tx",
		Long:  "abe-indexer is a application that index bitcoin tx",
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			ctx := context.Background()
			ctx = context.WithValue(ctx, model.ServerContextKey, handler.NewDefaultContext())
			cmd.SetContext(ctx)
		},
	}

	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(buildIndexCmd())
	return rootCmd
}

func buildIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start index tx service",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return handler.InterceptConfigsPreRunHandler(cmd)
		},
		Run: func(cmd *cobra.Command, _ []string) {
			err := handler.HandleIndexCmd(GetServerContextFromCmd(cmd), cmd)
			if err != nil {
				log.Error("start index tx service failed")
			}
		},
	}
	return cmd
}

// GetServerContextFromCmd returns a Context from a command or an empty Context
// if it has not been set.
func GetServerContextFromCmd(cmd *cobra.Command) *model.Context {
	if v := cmd.Context().Value(model.ServerContextKey); v != nil {
		serverCtxPtr := v.(*model.Context)
		return serverCtxPtr
	}

	return handler.NewDefaultContext()
}
