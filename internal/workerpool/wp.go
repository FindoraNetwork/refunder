package workerpool

import (
	"log"
	"sync"
)

type WorkerPool struct {
	wg        *sync.WaitGroup
	pool      chan func() error
	workerNum int
	logger    *log.Logger
}

func New(options ...func(*WorkerPool)) *WorkerPool {
	wp := &WorkerPool{
		wg:        new(sync.WaitGroup),
		pool:      make(chan func() error),
		workerNum: 1,
	}

	for _, o := range options {
		o(wp)
	}

	for i := 0; i < wp.workerNum; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}

	return wp
}

func WithPoolSize(size int) func(*WorkerPool) {
	return func(wp *WorkerPool) {
		wp.pool = make(chan func() error, size)
	}
}

func WithWorkerNum(num int) func(*WorkerPool) {
	return func(wp *WorkerPool) {
		wp.workerNum = num
	}
}

func WithLogger(logger *log.Logger) func(*WorkerPool) {
	return func(wp *WorkerPool) {
		wp.logger = logger
	}
}

func (wp *WorkerPool) worker() {
	defer wp.wg.Done()
	for job := range wp.pool {
		if err := job(); err != nil {
			log.Println(err)
		}
	}
}

func (wp *WorkerPool) PutJob(job func() error) {
	wp.pool <- job
}

func (wp *WorkerPool) Close() {
	close(wp.pool)
	wp.wg.Wait()
}
