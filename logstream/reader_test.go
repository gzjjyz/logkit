package logstream

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"os"
	"os/signal"
	"testing"
	"time"
)

func TestReader_Start(t *testing.T) {
	logger.InitLogger(logger.WithAppName("testreader"))

	var (
		readStream *Reader
		err        error
	)
	readStream, err = NewReader("", func(items []*PoppedMsgItem) error {
		for _, item := range items {
			fmt.Println(string(item.Data))
			fmt.Println("Retry at:", item.RetryAt)
			fmt.Println("Retry cnt:", item.RetryCnt, " now:", time.Now().Unix())
			if item.RetryCnt > 3 {
				readStream.ConfirmMsg(item.Topic, item.Seq, item.IdxOffset)
			}
		}
		fmt.Println(items[0].Topic+" batch consumed", len(items))
		return nil
	})
	if err != nil {
		panic(err)
	}

	go func() {
		signCh := make(chan os.Signal)
		signal.Notify(signCh)
		<-signCh
		readStream.Exit()
	}()

	readStream.Start()
}
