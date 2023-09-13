package main

import (
	"github.com/995933447/goconsole"
	"github.com/995933447/std-go/scan"
	"github.com/gzjjyz/srvlib/logger"
)

func init() {
	logger.InitLogger("LogStreamTool")
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
