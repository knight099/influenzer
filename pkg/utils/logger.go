package utils

import (
	"go.uber.org/zap"
)

var Logger *zap.Logger

func InitLogger() {
	var err error
	// Use production logger for now, can be configured based on env
	Logger, err = zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer Logger.Sync()
}
