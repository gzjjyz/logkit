//go:build windows
// +build windows

package logstream

import "github.com/gzjjyz/srvlib/utils"

var defaultCfgFilePath = utils.GetCurrentDir() + "logmeta.json"
