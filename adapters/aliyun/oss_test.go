package aliyun

import (
	"context"
	"sync"
	"testing"
)

func TestAdapterCloseStateIsRaceSafe(t *testing.T) {
	adapter := &Adapter{}
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 1000 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = adapter.Close(ctx)
		}()
		go func() {
			defer wg.Done()
			_ = adapter.isClosed()
		}()
	}
	wg.Wait()

	if !adapter.isClosed() {
		t.Fatal("expected adapter to be closed")
	}
}
