package cmd

import (
	"context"
	"os"

	"github.com/b2network/b2-indexer/internal/handler"
	"github.com/b2network/b2-indexer/internal/model"
	"github.com/b2network/b2-indexer/internal/types"
	"github.com/b2network/b2-indexer/pkg/log"
	"github.com/spf13/cobra"
)

const (
	FlagHome = "home"
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
		Use:   "b2-indexer",
		Short: "index tx",
		Long:  "b2-indexer is a application that index bitcoin tx",
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			ctx := context.Background()
			ctx = context.WithValue(ctx, types.ServerContextKey, handler.NewDefaultContext())
			cmd.SetContext(ctx)
		},
	}

	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(buildIndexCmd())
	rootCmd.AddCommand(buildHttpCmd())
	//rootCmd.AddCommand(sinohopeCmd.Sinohope())
	//rootCmd.AddCommand(gvsmCmd.Gvsm())
	//rootCmd.AddCommand(cryptoCmd.Crypto())
	return rootCmd
}

func buildIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start index tx service",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			home, err := cmd.Flags().GetString(FlagHome)
			if err != nil {
				return err
			}
			return handler.InterceptConfigsPreRunHandler(cmd, home)
		},
		Run: func(cmd *cobra.Command, _ []string) {
			err := handler.HandleIndexCmd(GetServerContextFromCmd(cmd), cmd)
			if err != nil {
				log.Error("start index tx service failed")
			}
		},
	}
	cmd.Flags().String(FlagHome, "./", "The application home directory")
	return cmd
}

// GetServerContextFromCmd returns a Context from a command or an empty Context
// if it has not been set.
func GetServerContextFromCmd(cmd *cobra.Command) *model.Context {
	if v := cmd.Context().Value(types.ServerContextKey); v != nil {
		serverCtxPtr := v.(*model.Context)
		return serverCtxPtr
	}

	return handler.NewDefaultContext()
}

func buildHttpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "http",
		Short: "start http service",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			home, err := cmd.Flags().GetString(FlagHome)
			if err != nil {
				return err
			}
			return handler.HTTPConfigsPreRunHandler(cmd, home)
		},
		Run: func(cmd *cobra.Command, _ []string) {
			db, err := handler.GetDBContextFromCmd(cmd)
			if err != nil {
				cmd.Println(err)
				return
			}
			err = handler.Run(cmd.Context(), GetServerContextFromCmd(cmd), db)
			if err != nil {
				log.Error("start http service failed")
			}
		},
	}
	cmd.Flags().String(FlagHome, "", "The application home directory")
	return cmd
}
