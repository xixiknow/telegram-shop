package payment

import "time"

// randSeed 返回一个时间相关的随机种子。
func randSeed() int64 {
	return time.Now().UnixNano()
}
