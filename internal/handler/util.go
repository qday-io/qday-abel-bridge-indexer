package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/b2network/b2-indexer/config"
	"github.com/b2network/b2-indexer/internal/model"
	"github.com/b2network/b2-indexer/internal/types"
	logger "github.com/b2network/b2-indexer/pkg/log"
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

func InterceptConfigsPreRunHandler(cmd *cobra.Command, home string) error {
	cfg, err := config.LoadConfig(home)
	if err != nil {
		return err
	}
	if home != "" {
		cfg.RootDir = home
	}

	bitcoinCfg, err := config.LoadBitcoinConfig(home)
	if err != nil {
		return err
	}

	//the version no db,next open
	db, err := NewDB(cfg)
	if err != nil {
		return err
	}

	//set db to context
	ctx := context.WithValue(cmd.Context(), types.DBContextKey, db)
	cmd.SetContext(ctx)

	logger.Init(cfg.LogLevel, cfg.LogFormat)
	serverCtx := NewContext(cfg, bitcoinCfg)
	return SetCmdServerContext(cmd, serverCtx)
}

// GetServerContextFromCmd returns a Context from a command or an empty Context
// if it has not been set.
func GetServerContextFromCmd(cmd *cobra.Command) *model.Context {
	if v := cmd.Context().Value(types.ServerContextKey); v != nil {
		serverCtxPtr := v.(*model.Context)
		return serverCtxPtr
	}

	return NewDefaultContext()
}

// SetCmdServerContext sets a command's Context value to the provided argument.
func SetCmdServerContext(cmd *cobra.Command, serverCtx *model.Context) error {
	v := cmd.Context().Value(types.ServerContextKey)
	if v == nil {
		return errors.New("server context not set")
	}

	serverCtxPtr := v.(*model.Context)
	*serverCtxPtr = *serverCtx

	return nil
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

func NewHTTPContext(httpCfg *config.HTTPConfig, bitcoinCfg *config.BitcoinConfig) *model.Context {
	return &model.Context{
		HTTPConfig:    httpCfg,
		BitcoinConfig: bitcoinCfg,
	}
}

func HTTPConfigsPreRunHandler(cmd *cobra.Command, home string) error {
	cfg, err := config.LoadConfig(home)
	if err != nil {
		return err
	}
	if home != "" {
		cfg.RootDir = home
	}

	httpCfg, err := config.LoadHTTPConfig(home)
	if err != nil {
		return err
	}

	bitcoinCfg, err := config.LoadBitcoinConfig(home)
	if err != nil {
		return err
	}
	db, err := NewDB(cfg)
	if err != nil {
		return err
	}

	// set db to context
	ctx := context.WithValue(cmd.Context(), types.DBContextKey, db)
	cmd.SetContext(ctx)
	serverCtx := NewHTTPContext(httpCfg, bitcoinCfg)
	return SetCmdServerContext(cmd, serverCtx)
}
