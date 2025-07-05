package code

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
)

// tempBuffer 是一个用于存储和监控终端输出的缓冲区
type tempBuffer struct {
	mu           sync.Mutex
	buffer       []byte                   // 存储所有输出
	watchers     map[string]*roundWatcher // 存储每个 prompt 的观察者
	enterableStr string                   // 标识可输入状态的字符串
	f            *os.File
}

// roundWatcher 表示一个观察特定 prompt 输出的观察者
type roundWatcher struct {
	key    string        // prompt 的唯一标识
	buffer *bytes.Buffer // 存储针对此 prompt 的输出
	done   bool          // 是否已完成
	mu     sync.Mutex    // 互斥锁
}

// newTempBuffer 创建一个新的 tempBuffer
func newTempBuffer() *tempBuffer {
	return &tempBuffer{
		buffer:       make([]byte, 0),
		watchers:     make(map[string]*roundWatcher),
		enterableStr: ">   ", // Gemini 的提示符
	}
}

// Write 实现 io.Writer 接口，将数据写入缓冲区并通知所有观察者
func (t *tempBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 追加到主缓冲区
	t.buffer = append(t.buffer, p...)
	lastIndex := strings.LastIndex(string(t.buffer), "[2J[3J[H")
	buffer := t.buffer
	if lastIndex > 0 {
		buffer = t.buffer[lastIndex:]
	}

	// 通知所有观察者
	for _, watcher := range t.watchers {
		watcher.mu.Lock()
		if !watcher.done {
			endIdx := strings.LastIndex(string(buffer), t.enterableStr)
			questionIdx := strings.LastIndex(string(buffer), watcher.key)
			if endIdx > 0 && questionIdx > 0 && questionIdx < endIdx {
				buf := buffer[questionIdx+len(watcher.key) : endIdx]
				start := strings.Index(string(buf), "╯")
				end := strings.LastIndex(string(buf), "Using 1 GEMINI.md file")
				if start > 0 && end > 0 {
					watcher.buffer.Write(buf[start+3 : end])
				} else {
					watcher.buffer.Write(buf)
				}
				watcher.done = true
			}
		}
		watcher.mu.Unlock()
	}

	return len(p), nil
}

// Watch 创建一个新的观察者来监控特定 prompt 的输出
func (t *tempBuffer) Watch(key string) io.Reader {
	t.mu.Lock()
	defer t.mu.Unlock()

	keyPrefix := key
	if len(keyPrefix) > 10 {
		keyPrefix = keyPrefix[:10]
	}

	searchKey := "> " + keyPrefix

	watcher := &roundWatcher{
		key:    searchKey,
		buffer: new(bytes.Buffer),
		done:   false,
	}

	t.watchers[key] = watcher
	return &roundReader{watcher: watcher}
}

// Enterable 检查缓冲区是否包含可输入状态的标识
func (t *tempBuffer) Enterable() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 检查最后 100 个字符，避免检查整个缓冲区
	lastN := 1000
	if len(t.buffer) < lastN {
		lastN = len(t.buffer)
	}

	lastPart := string(t.buffer[len(t.buffer)-lastN:])
	return strings.Contains(lastPart, t.enterableStr)
}

// roundReader 实现了 io.Reader 接口，用于读取 roundWatcher 的输出
type roundReader struct {
	watcher *roundWatcher
	offset  int // 读取偏移量
}

// Read 实现 io.Reader 接口
func (r *roundReader) Read(p []byte) (n int, err error) {
	r.watcher.mu.Lock()
	defer r.watcher.mu.Unlock()

	// 如果 watcher 已完成且已读取所有内容，返回 EOF
	if r.watcher.done && r.offset >= r.watcher.buffer.Len() {
		return 0, io.EOF
	}

	// 从 buffer 中读取数据
	data := r.watcher.buffer.Bytes()
	if r.offset < len(data) {
		n = copy(p, data[r.offset:])
		r.offset += n
		return n, nil
	}

	// 如果尚未读取到数据但 watcher 未完成，返回零值让调用者稍后重试
	return 0, nil
}
