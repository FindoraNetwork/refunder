package workerpool_test

import (
	"errors"
	"log"
	"os"
	"testing"

	"github.com/FindoraNetwork/refunder/internal/workerpool"
)

func Test_WorkerPool(t *testing.T) {
	wp := workerpool.New(
		workerpool.WithPoolSize(0),
		workerpool.WithWorkerNum(1),
		workerpool.WithLogger(log.New(os.Stdout, "Test_WorkerPool", log.Lmsgprefix)),
	)
	for i := 0; i < 9; i++ {
		wp.PutJob(func() error {
			return errors.New("error")
		})
	}
	wp.Close()
}
