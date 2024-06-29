package model

import (
	"github.com/b2network/b2-indexer/config"
)

type Context struct {
	// Viper         *viper.Viper
	Config        *config.Config
	BitcoinConfig *config.BitcoinConfig
	//HTTPConfig    *config.HTTPConfig
	// Logger        logger.Logger
	// Db *gorm.DB
}
