package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/qday-io/qday-abel-bridge-indexer/config"
	"github.com/qday-io/qday-abel-bridge-indexer/internal/model"
	logger "github.com/qday-io/qday-abel-bridge-indexer/pkg/log"
	"github.com/spf13/cobra"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
)

// type serverContext string

// // ServerContextKey defines the context key used to retrieve a server.Context from
// // a command's Context.
// const (
// 	ServerContextKey = serverContext("server.context")
// 	DBContextKey     = serverContext("db.context")
// )

// ErrorCode contains the exit code for server exit.
type ErrorCode struct {
	Code int
}

func (e ErrorCode) Error() string {
	return strconv.Itoa(e.Code)
}

func NewDefaultContext() *model.Context {
	return NewContext(
		config.DefaultConfig(),
		config.DefaultBitcoinConfig(),
	)
}

func NewContext(cfg *config.Config, btcCfg *config.BitcoinConfig) *model.Context {
	return &model.Context{
		Config:        cfg,
		BitcoinConfig: btcCfg,
	}
}

// InterceptConfigsPreRunHandler initializes and sets up the application context before command execution.
// It loads configurations, establishes database connection, initializes logger, and sets up server context.
func InterceptConfigsPreRunHandler(cmd *cobra.Command) error {
	// Step 1: Load all configurations at once
	appConfig, err := config.LoadAppConfig()
	if err != nil {
		return fmt.Errorf("failed to load application config: %w", err)
	}

	// Step 2: Initialize logger with configuration
	logger.Init(appConfig.LogLevel, appConfig.LogFormat)

	// Step 3: Establish database connection
	db, err := NewDBFromAppConfig(appConfig)
	if err != nil {
		return fmt.Errorf("failed to establish database connection: %w", err)
	}

	// Step 4: Create server context with all configurations
	serverCtx := NewContextFromAppConfig(appConfig)

	// Step 5: Set up command context with database and server context
	if err := setupCommandContext(cmd, db, serverCtx); err != nil {
		return fmt.Errorf("failed to setup command context: %w", err)
	}

	return nil
}

// setupCommandContext sets up the command context with database and server context.
func setupCommandContext(cmd *cobra.Command, db *gorm.DB, serverCtx *model.Context) error {
	// Get the current context
	currentCtx := cmd.Context()
	if currentCtx == nil {
		return errors.New("command context is nil")
	}

	// Set database context
	dbCtx := context.WithValue(currentCtx, model.DBContextKey, db)

	// Set server context
	serverCtxPtr := currentCtx.Value(model.ServerContextKey)
	if serverCtxPtr == nil {
		return errors.New("server context not set in command")
	}

	// Update server context
	serverCtxValue := serverCtxPtr.(*model.Context)
	*serverCtxValue = *serverCtx

	// Set the updated context back to command
	cmd.SetContext(dbCtx)

	return nil
}

// GetServerContextFromCmd returns a Context from a command or an empty Context
// if it has not been set.
func GetServerContextFromCmd(cmd *cobra.Command) *model.Context {
	if v := cmd.Context().Value(model.ServerContextKey); v != nil {
		serverCtxPtr := v.(*model.Context)
		return serverCtxPtr
	}

	return NewDefaultContext()
}

// NewContextFromAppConfig creates a server context from unified app config
func NewContextFromAppConfig(appConfig *config.AppConfig) *model.Context {
	// Convert AppConfig to separate Config and BitcoinConfig for backward compatibility
	cfg := &config.Config{
		RootDir:                 appConfig.RootDir,
		LogLevel:                appConfig.LogLevel,
		LogFormat:               appConfig.LogFormat,
		DatabaseSource:          appConfig.DatabaseSource,
		DatabaseMaxIdleConns:    appConfig.DatabaseMaxIdleConns,
		DatabaseMaxOpenConns:    appConfig.DatabaseMaxOpenConns,
		DatabaseConnMaxLifetime: appConfig.DatabaseConnMaxLifetime,
	}

	bitcoinCfg := &config.BitcoinConfig{
		NetworkName:                      appConfig.NetworkName,
		RPCHost:                          appConfig.RPCHost,
		RPCPort:                          appConfig.RPCPort,
		RPCUser:                          appConfig.RPCUser,
		RPCPass:                          appConfig.RPCPass,
		DisableTLS:                       appConfig.DisableTLS,
		WalletName:                       appConfig.WalletName,
		EnableIndexer:                    appConfig.EnableIndexer,
		IndexerListenAddress:             appConfig.IndexerListenAddress,
		IndexerListenTargetConfirmations: appConfig.IndexerListenTargetConfirmations,
		Bridge:                           appConfig.Bridge,
	}

	return &model.Context{
		Config:        cfg,
		BitcoinConfig: bitcoinCfg,
	}
}

// NewDBFromAppConfig creates a new database connection from unified app config
func NewDBFromAppConfig(appConfig *config.AppConfig) (*gorm.DB, error) {
	// Convert AppConfig to Config for backward compatibility
	cfg := &config.Config{
		RootDir:                 appConfig.RootDir,
		LogLevel:                appConfig.LogLevel,
		LogFormat:               appConfig.LogFormat,
		DatabaseSource:          appConfig.DatabaseSource,
		DatabaseMaxIdleConns:    appConfig.DatabaseMaxIdleConns,
		DatabaseMaxOpenConns:    appConfig.DatabaseMaxOpenConns,
		DatabaseConnMaxLifetime: appConfig.DatabaseConnMaxLifetime,
	}

	return NewDB(cfg)
}

// NewDB creates a new database connection.
// default use postgres driver
func NewDB(cfg *config.Config) (*gorm.DB, error) {

	var DB *gorm.DB
	var err error
	for i := 0; i < 2; i++ {
		// waiting for db server start complete
		time.Sleep(10 * time.Second)

		DB, err = gorm.Open(postgres.Open(cfg.DatabaseSource), &gorm.Config{
			Logger: gormlog.Default.LogMode(gormlog.Info),
		})
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return nil, err
	}
	// set db conn limit
	sqlDB.SetMaxIdleConns(cfg.DatabaseMaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.DatabaseMaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.DatabaseConnMaxLifetime) * time.Second)
	return DB, nil
}
