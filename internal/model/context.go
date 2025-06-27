package model

import (
	"github.com/qday-io/qday-abel-bridge-indexer/config"
)

type Context struct {
	// Viper         *viper.Viper
	Config        *config.Config
	BitcoinConfig *config.BitcoinConfig
	//HTTPConfig    *config.HTTPConfig
	// Logger        logger.Logger
	// Db *gorm.DB
}
