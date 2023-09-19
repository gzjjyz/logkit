package main

import (
	"encoding/binary"
	"fmt"
	"github.com/gzjjyz/logger"
	"io"
	"os"
	"strings"
)

func CheckIdxCorruption(baseDir string) error {
	var checkByDir func(dir string) error

	checkByDir = func(dir string) error {
		dir = strings.TrimRight(dir, string(os.PathSeparator)) + "/"
		files, err := os.ReadDir(dir)
		if err != nil {
			logger.Errorf(err.Error())
			return err
		}

		for _, file := range files {
			if file.IsDir() {
				if err := checkByDir(dir + file.Name()); err != nil {
					logger.Errorf(err.Error())
					return err
				}
			}
			if !strings.HasSuffix(file.Name(), ".idx") {
				continue
			}

			fileInfo, err := file.Info()
			if err != nil {
				logger.Errorf(err.Error())
				return err
			}

			if fileInfo.Size()%32 > 0 {
				return fmt.Errorf("file %s happen corrupted", dir+fileInfo.Name())
			}

			idxFp, err := os.Open(dir + file.Name())
			if err != nil {
				return err
			}

			dataFp, err := os.Open(dir + strings.ReplaceAll(file.Name(), ".idx", ".dat"))
			if err != nil {
				return err
			}

			idxBytes, err := io.ReadAll(idxFp)
			if err != nil {
				return err
			}

			dataBytes, err := io.ReadAll(dataFp)
			if err != nil {
				return err
			}

			bin := binary.LittleEndian
			var idxCnt uint32
			for {
				idxCnt++
				idxB := idxBytes[0:32]
				idxBytes = idxBytes[32:]
				if len(idxBytes) <= 0 {
					break
				}

				beginMark := idxB[:2]
				endMark := idxB[30:]
				if bin.Uint16(beginMark) != uint16(0x1234) || bin.Uint16(endMark) != uint16(0x5678) {
					logger.Warn(
						"bin.Uint16(beginMark) != uint16(0x1234) is %v, bin.Uint16(endMark) != uint16(0x1234) is %v",
						bin.Uint16(beginMark) != uint16(0x1234),
						bin.Uint16(endMark) != uint16(0x1234),
					)
					return fmt.Errorf("file %s happen corrupted, wrong bytes offset:%d end:%d", dir+idxFp.Name(), (idxCnt-1)*32, idxCnt*32)
				}

				offset := bin.Uint32(idxB[2+4 : 2+8])
				dataLen := bin.Uint32(idxB[2+8 : 2+12])

				if offset > uint32(len(dataBytes)) || offset+dataLen >= uint32(len(dataBytes)) {
					return fmt.Errorf("file %s happen corrupted, wrong bytes offset:%d end:%d", dir+dataFp.Name(), offset, offset+dataLen)
				}

				dataB := dataBytes[offset : offset+dataLen]
				beginMark = dataB[:2]
				endMark = dataB[dataLen-2:]
				if bin.Uint16(beginMark) != uint16(0x1234) || bin.Uint16(endMark) != uint16(0x5678) {
					return fmt.Errorf("file %s happen corrupted, wrong bytes offset:%d end:%d", dir+dataFp.Name(), offset, offset+dataLen)
				}
			}
		}

		return nil
	}

	if err := checkByDir(baseDir); err != nil {
		logger.Errorf(err.Error())
		return err
	}

	return nil
}
