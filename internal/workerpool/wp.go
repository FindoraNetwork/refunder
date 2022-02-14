package workerpool

import (
	"log"
	"sync"
)

// WorkerPool provides a simple way to use worker pool pattern
// TODO: if the more detailed operations are needed we can change to use this instead:
// https://github.com/honestbee/jobq
type WorkerPool struct {
	wg        *sync.WaitGroup
	pool      chan func() error
	workerNum int
	logger    *log.Logger
}

// New returns the WorkerPool structure
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

// WithPoolSize provides an option pattern to setup the buffered channel pool size
func WithPoolSize(size int) func(*WorkerPool) {
	return func(wp *WorkerPool) {
		wp.pool = make(chan func() error, size)
	}
}

// WithWorkerNum provides an option pattern to setup the number of workers
func WithWorkerNum(num int) func(*WorkerPool) {
	return func(wp *WorkerPool) {
		wp.workerNum = num
	}
}

// WithLogger provides an option pattern to setup the logger for worker to print out message
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

// PutJob inserts a job function into the pool for worker to consume
func (wp *WorkerPool) PutJob(job func() error) {
	wp.pool <- job
}

// Close closes the pool channel then stops all the workers and waits for each worker to finish their current job
func (wp *WorkerPool) Close() {
	close(wp.pool)
	wp.wg.Wait()
}
