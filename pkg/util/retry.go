package util

import (
	"fmt"
	"log"
	"time"
)

// Retry 执行有重试机制的操作
func Retry(attempts int, sleep time.Duration, operation func() error) error {
	var err error

	for i := 0; i < attempts; i++ {
		err = operation()
		if err == nil {
			return nil
		}

		if i < attempts-1 {
			log.Printf("操作失败(尝试 %d/%d): %v - 将在 %v 后重试", i+1, attempts, err, sleep)
			time.Sleep(sleep)
			sleep *= 2 // 指数退避策略
		}
	}

	return fmt.Errorf("在 %d 次尝试后操作失败: %v", attempts, err)
}
