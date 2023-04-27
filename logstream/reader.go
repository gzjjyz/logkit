package logstream

import (
	"errors"
	"github.com/995933447/confloader"
	"github.com/gzjjyz/srvlib/logger"
	"github.com/gzjjyz/srvlib/utils"
	"sync"
	"time"
)

const (
	readerMemMaxBytes          = 1024 * 1024 * 1024 * 2
	readerMaxConcurrentForward = 300
	// os thread(8m) + consumer max popped(2m) = 12m
	readerSchedWorkerMemBytes = 1024 * 1024 * 12
)

func NewReader(cfgFilePath string, forwarder ForwardFunc) (*Reader, error) {
	if !logger.HasInit() {
		return nil, errors.New("logger not init")
	}

	if cfgFilePath == "" {
		cfgFilePath = defaultCfgFilePath
	}

	var (
		cfg Cfg
		err error
	)
	cfgLoader := confloader.NewLoader(cfgFilePath, refreshCfgInterval, &cfg)
	if err = cfgLoader.Load(); err != nil {
		return nil, err
	}

	reader := &Reader{
		baseDir:                cfg.BaseDir,
		topics:                 NewTopicSet(),
		blackTopics:            NewTopicSet(),
		whiteTopics:            NewTopicSet(),
		maxConcurrentForward:   cfg.MaxConcurrentForward,
		forwarder:              forwarder,
		schedCh:                make(chan *Consumer),
		retryCh:                make(chan []*PoppedMsgItem),
		forwardCh:              make(chan []*PoppedMsgItem),
		topicConsumerMap:       map[string]*Consumer{},
		exitOneSchedWorkerCh:   make(chan struct{}),
		exitOneForwardWorkerCh: make(chan struct{}),
	}

	if cfg.MemMaxSize != "" {
		reader.memMaxBytes, err = parseMemSizeStrToBytes(cfg.MemMaxSize)
		if err != nil {
			return nil, err
		}
	}

	if reader.memMaxBytes == 0 {
		reader.memMaxBytes = readerMemMaxBytes
	}

	if reader.maxConcurrentForward == 0 {
		reader.maxConcurrentForward = readerMaxConcurrentForward
	}

	reader.whiteTopics.reset(cfg.WhiteTopics)
	reader.blackTopics.reset(cfg.BlackTopics)

	if err = reader.init(); err != nil {
		return nil, err
	}

	utils.ProtectGo(func() {
		watchReaderCfg(reader, cfgLoader, &cfg)
	})

	return reader, nil
}

func watchReaderCfg(reader *Reader, cfgLoader *confloader.Loader, cfg *Cfg) {
	refreshCfgErr := make(chan error)
	utils.ProtectGo(func() {
		refreshCfgTk := time.NewTicker(refreshCfgInterval + time.Second)
		defer refreshCfgTk.Stop()
		for {
			select {
			case err := <-refreshCfgErr:
				logger.Errorf(err.Error())
			case <-refreshCfgTk.C:
				reader.accessFickleMu.Lock()
				if reader.baseDir != cfg.BaseDir {
					reader.removeAllTopics()
					reader.baseDir = cfg.BaseDir
				}

				var memMaxBytes uint32
				if cfg.MemMaxSize != "" {
					var err error
					memMaxBytes, err = parseMemSizeStrToBytes(cfg.MemMaxSize)
					if err != nil {
						logger.Errorf(err.Error())
						break
					}
				}
				if memMaxBytes == 0 {
					memMaxBytes = readerMemMaxBytes
				}
				if memMaxBytes != reader.memMaxBytes {
					reader.waitExpandMemBytes = int64(memMaxBytes) - int64(reader.memMaxBytes)
					reader.memMaxBytes = memMaxBytes
				}
				maxConcurrentForward := cfg.MaxConcurrentForward
				if maxConcurrentForward == 0 {
					maxConcurrentForward = readerMaxConcurrentForward
				}
				if maxConcurrentForward != reader.maxConcurrentForward {
					reader.waitExpandConcurrentForward = int32(cfg.MaxConcurrentForward) - int32(reader.maxConcurrentForward)
					reader.maxConcurrentForward = cfg.MaxConcurrentForward
				}

				reader.whiteTopics.reset(cfg.WhiteTopics)
				reader.blackTopics.reset(cfg.BlackTopics)
				if err := reader.init(); err != nil {
					logger.Errorf(err.Error())
				}
				reader.accessFickleMu.Unlock()
			}
		}
	})
	cfgLoader.WatchToLoad(refreshCfgErr)
}

type ForwardFunc func([]*PoppedMsgItem) error

type Reader struct {
	// config properties
	baseDir                     string
	topics                      *TopicSet
	blackTopics                 *TopicSet
	whiteTopics                 *TopicSet
	topicConsumerMap            map[string]*Consumer
	memMaxBytes                 uint32
	waitExpandMemBytes          int64
	maxConcurrentForward        uint32
	waitExpandConcurrentForward int32
	forwarder                   ForwardFunc

	// runtime properties
	accessFickleMu         sync.RWMutex
	schedCh                chan *Consumer        // chan use to schedule consumer to consume msg
	forwardCh              chan []*PoppedMsgItem // chan use to transfer messages to net
	retryCh                chan []*PoppedMsgItem // chan use to retry failed messages
	exitOneSchedWorkerCh   chan struct{}
	exitOneForwardWorkerCh chan struct{}
}

func (r *Reader) init() error {
	hasBlackTopics := r.blackTopics.size() > 0
	hasWhiteTopics := r.whiteTopics.size() > 0
	topics, err := scanDirToParseTopics(r.baseDir, func(topic string) bool {
		if hasWhiteTopics {
			return r.whiteTopics.exist(topic)
		}

		if hasBlackTopics {
			return !r.blackTopics.exist(topic)
		}

		return true
	})
	if err != nil {
		return err
	}

	oldTopics := r.topics.list()

	r.topics.reset(topics)

	for _, old := range oldTopics {
		if r.topics.exist(old) {
			continue
		}

		consumer, ok := r.topicConsumerMap[old]
		if !ok {
			continue
		}

		delete(r.topicConsumerMap, old)
		utils.ProtectGo(func() {
			consumer.unsubscribe()
		})
	}

	err = r.topics.walk(func(topic string) (bool, error) {
		consumer, ok := r.topicConsumerMap[topic]
		if !ok {
			consumer, err = newConsumer(r, topic)
			if err != nil {
				return false, err
			}

			utils.ProtectGo(func() {
				if err = consumer.subscribe(); err != nil {
					logger.Errorf(err.Error())
				}
			})

			r.topicConsumerMap[topic] = consumer
		}

		return true, nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *Reader) removeAllTopics() {
	for _, topic := range r.topics.list() {
		r.topics.del(topic)

		consumer, ok := r.topicConsumerMap[topic]
		if !ok {
			continue
		}

		delete(r.topicConsumerMap, topic)
		utils.ProtectGo(func() {
			consumer.unsubscribe()
		})
	}
}

func (r *Reader) expandWorkerPool() {
	if r.waitExpandMemBytes < 0 {
		removeWorkerNum := int((-r.waitExpandMemBytes) / readerSchedWorkerMemBytes)
		for i := 0; i < removeWorkerNum; i++ {
			utils.ProtectGo(func() {
				r.exitOneSchedWorkerCh <- struct{}{}
			})
		}
	} else if r.waitExpandMemBytes > 0 {
		addWorkerNum := int(r.waitExpandMemBytes / readerSchedWorkerMemBytes)
		for i := 0; i < addWorkerNum; i++ {
			r.runSchedWorker()
		}
	}
	r.waitExpandMemBytes = 0

	if r.waitExpandConcurrentForward < 0 {
		for i := r.waitExpandConcurrentForward; i < 0; i++ {
			utils.ProtectGo(func() {
				r.exitOneForwardWorkerCh <- struct{}{}
			})
		}
	} else if r.waitExpandConcurrentForward > 0 {
		var i int32
		for ; i < r.waitExpandConcurrentForward; i++ {
			r.runForwardWorker()
		}
	}
	r.waitExpandConcurrentForward = 0
}

func (r *Reader) runSchedWorker() {
	utils.ProtectGo(func() {
		for {
			select {
			case <-r.exitOneSchedWorkerCh:
				logger.Debug("exited sched worker")
				goto out
			case consumer := <-r.schedCh:
				r.accessFickleMu.RLock()
				// topic already stopped
				if _, ok := r.topicConsumerMap[consumer.topic]; !ok {
					continue
				}
				r.accessFickleMu.RUnlock()
				popped, ok, err := consumer.consumeBatch()
				if err != nil {
					logger.Errorf(err.Error())
					continue
				}
				if !ok {
					continue
				}
				r.forwardCh <- popped
			}
		}
	out:
		return
	})
}

func (r *Reader) createSchedWorkerPool() {
	workerNum := r.memMaxBytes / readerSchedWorkerMemBytes
	var i uint32
	for ; i < workerNum; i++ {
		r.runSchedWorker()
	}
}

func (r *Reader) runForwardWorker() {
	utils.ProtectGo(func() {
		for {
			select {
			case <-r.exitOneForwardWorkerCh:
				logger.Debug("exited forward")
				goto out
			case msgItems := <-r.forwardCh:
				if len(msgItems) == 0 {
					continue
				}
				r.accessFickleMu.RLock()
				// topic already stopped
				if _, ok := r.topicConsumerMap[msgItems[0].Topic]; !ok {
					continue
				}
				r.accessFickleMu.RUnlock()
				if err := r.doForward(msgItems); err != nil {
					logger.Errorf(err.Error())
					r.retryCh <- msgItems
				}
			}
		}
	out:
		return
	})
}

func (r *Reader) createForwardWorkerPool() {
	var i uint32
	for ; i < r.maxConcurrentForward; i++ {
		r.runForwardWorker()
	}
}

func (r *Reader) doForward(msgItems []*PoppedMsgItem) error {
	return r.forwarder(msgItems)
}

// Start scheduling
func (r *Reader) Start() {
	r.createSchedWorkerPool()
	r.createForwardWorkerPool()

	// schedule retry messages and dynamically controller worker pool size
	expandWorkerPoolTk := time.NewTicker(time.Second * 3)
	defer expandWorkerPoolTk.Stop()
	var (
		retryMsgItemsList [][]*PoppedMsgItem
		retryMsgItems     []*PoppedMsgItem
		forwardCh         chan []*PoppedMsgItem
	)
	for {
		if len(retryMsgItemsList) > 0 && retryMsgItems == nil {
			forwardCh = r.forwardCh
			retryMsgItems = retryMsgItemsList[0]
			retryMsgItemsList = retryMsgItemsList[1:]
		}

		select {
		case forwardCh <- retryMsgItems:
			if len(retryMsgItemsList) == 0 {
				retryMsgItems = nil
				forwardCh = nil
			}
		case msgItems := <-r.retryCh:
			retryMsgItemsList = append(retryMsgItemsList, msgItems)
		case <-expandWorkerPoolTk.C:
			r.accessFickleMu.RLock()
			r.expandWorkerPool()
			r.accessFickleMu.RUnlock()
		}
	}
}

func (r *Reader) ConfirmMsg(topic string, seq uint64, idxOffset uint32) {
	consumer, ok := r.topicConsumerMap[topic]
	if !ok {
		return
	}
	consumer.confirmMsg(seq, idxOffset)
}
