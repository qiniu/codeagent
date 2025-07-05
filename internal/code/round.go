package code

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// tempBuffer 是一个用于存储和监控终端输出的缓冲区
type tempBuffer struct {
	mu           sync.Mutex
	buffer       []byte                   // 存储所有输出
	watchers     map[string]*roundWatcher // 存储每个 prompt 的观察者
	enterableStr string                   // 标识可输入状态的字符串
}

// roundWatcher 表示一个观察特定 prompt 输出的观察者
type roundWatcher struct {
	key        string        // prompt 的唯一标识
	buffer     *bytes.Buffer // 存储针对此 prompt 的输出
	startIndex int           // 开始索引位置
	done       bool          // 是否已完成
	mu         sync.Mutex    // 互斥锁
}

// newTempBuffer 创建一个新的 tempBuffer
func newTempBuffer() *tempBuffer {
	return &tempBuffer{
		buffer:       make([]byte, 0),
		watchers:     make(map[string]*roundWatcher),
		enterableStr: ">   Type your message", // Gemini 的提示符
	}
}

// Write 实现 io.Writer 接口，将数据写入缓冲区并通知所有观察者
func (t *tempBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 追加到主缓冲区
	t.buffer = append(t.buffer, p...)

	// 通知所有观察者
	for _, watcher := range t.watchers {
		watcher.mu.Lock()
		if !watcher.done {
			// 如果 watcher 的开始索引在 buffer 内
			if watcher.startIndex < len(t.buffer) {
				// 计算此 watcher 尚未接收的数据
				newData := t.buffer[watcher.startIndex:]
				watcher.buffer.Write(newData)
				watcher.startIndex = len(t.buffer)

				// 检查是否包含可输入状态标识
				if strings.Contains(watcher.buffer.String(), t.enterableStr) {
					watcher.done = true
				}
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

	watcher := &roundWatcher{
		key:        key,
		buffer:     new(bytes.Buffer),
		startIndex: len(t.buffer), // 从当前缓冲区末尾开始观察
		done:       false,
	}

	t.watchers[key] = watcher
	return &roundReader{watcher: watcher}
}

// Enterable 检查缓冲区是否包含可输入状态的标识
func (t *tempBuffer) Enterable() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 检查最后 100 个字符，避免检查整个缓冲区
	lastN := 100
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
