package main

import (
	"github.com/995933447/goconsole"
	"github.com/995933447/std-go/scan"
	"github.com/gzjjyz/logger"
)

func init() {
	logger.InitLogger(logger.WithAppName("LogStreamTool"))
}

func main() {
	goconsole.Register("CheckIdxCorrupt", "-d [data dir path]", func() {
		err := CheckIdxCorruption(scan.OptStrDefault("d", ""))
		if err != nil {
			logger.Errorf("err:%v", err.Error())
		}
	})
	goconsole.Run()
}
