package wsapp

import (
	"sync"
	"time"
)

// assistantDeltaCoalescer 合并高频 assistant 文本 delta。
type assistantDeltaCoalescer struct {
	mu      sync.Mutex
	window  time.Duration
	pending string
	timer   *time.Timer
	closed  bool
	flush   func(delta string)
}

// newAssistantDeltaCoalescer 使用 window 和 flush 参数创建文本 delta 合并器。
func newAssistantDeltaCoalescer(window time.Duration, flush func(delta string)) *assistantDeltaCoalescer {
	if window <= 0 {
		window = 60 * time.Millisecond
	}
	return &assistantDeltaCoalescer{window: window, flush: flush}
}

// Add 使用 delta 参数追加待合并文本。
func (c *assistantDeltaCoalescer) Add(delta string) {
	if delta == "" {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.pending += delta
	if c.timer == nil {
		c.timer = time.AfterFunc(c.window, c.flushTimer)
	}
	c.mu.Unlock()
}

// Flush 立即发送当前待合并文本。
func (c *assistantDeltaCoalescer) Flush() {
	c.flushPending(false)
}

// Close 发送剩余文本并关闭合并器。
func (c *assistantDeltaCoalescer) Close() {
	c.flushPending(true)
}

// flushTimer 处理定时器触发的发送。
func (c *assistantDeltaCoalescer) flushTimer() {
	c.flushPending(false)
}

// flushPending 使用 closing 参数发送 pending 文本。
func (c *assistantDeltaCoalescer) flushPending(closing bool) {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if closing {
		c.closed = true
	}
	delta := c.pending
	c.pending = ""
	c.mu.Unlock()

	if delta != "" && c.flush != nil {
		c.flush(delta)
	}
}
