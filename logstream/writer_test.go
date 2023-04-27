package logstream

import (
	"fmt"
	"github.com/gzjjyz/srvlib/logger"
	"testing"
)

func TestWriter_Write(t *testing.T) {
	logger.InitLogger("logstreamtest")
	writer, _ := NewWriter("")
	for i := 0; i < 30; i++ {
		writer.Write("test_topic", []byte(fmt.Sprintf("hello world:%d times", i)))
	}
}

func TestWriter_Flush(t *testing.T) {
	logger.InitLogger("logstreamtest")
	writer, err := NewWriter("")
	if err != nil {
		t.Log(err)
		return
	}
	for i := 0; i < 25; i++ {
		writer.Write("test_topic", []byte(fmt.Sprintf("hello world:%d times", i)))
	}
	writer.Flush()
}

func TestWriter_Exit(t *testing.T) {
	writer, _ := NewWriter("")
	for i := 0; i < 1000; i++ {
		writer.Write("test_topic", []byte(fmt.Sprintf("hello world:%d times", i)))
	}
	writer.Exit()
}

func TestWriter_Stop(t *testing.T) {
	writer, _ := NewWriter("")
	for i := 0; i < 10000; i++ {
		writer.Write("test_topic", []byte(fmt.Sprintf("hello world:%d times", i)))
	}
	writer.Stop()
}

func TestWriter_Resume(t *testing.T) {
	writer, _ := NewWriter("")
	for i := 0; i < 10000; i++ {
		writer.Write("test_topic", []byte(fmt.Sprintf("hello world:%d times", i)))
	}
	writer.Stop()
	err := writer.Resume()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		writer.Write("test_topic", []byte(fmt.Sprintf("hello world:%d times", i)))
	}
	writer.Exit()
	err = writer.Resume()
	if err != nil {
		t.Fatal(err)
	}
}
