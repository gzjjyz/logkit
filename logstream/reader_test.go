package logstream

import (
	"fmt"
	"github.com/gzjjyz/srvlib/logger"
	"testing"
	"time"
)

func TestReader_Start(t *testing.T) {
	logger.InitLogger("logstreamtest")
	var (
		readStream *Reader
		err        error
	)
	readStream, err = NewReader("", func(items []*PoppedMsgItem) error {
		time.Sleep(time.Second)
		//return errors.New("mimic err")

		fmt.Println("consume " + items[0].Topic)
		for _, item := range items {
			fmt.Println(string(item.Data))
			readStream.ConfirmMsg(item.Topic, item.Seq, item.IdxOffset)
		}
		fmt.Println(items[0].Topic+" batch consumed", len(items))
		return nil
	})
	if err != nil {
		panic(err)
	}

	readStream.Start()
}
