package main

import (
	"github.com/gzjjyz/logger"
	"testing"
)

func TestCheckIdxCorruption(t *testing.T) {
	err := CheckIdxCorruption("D:\\test_topic\\")
	if err != nil {
		logger.Errorf(err.Error())
	}
}
