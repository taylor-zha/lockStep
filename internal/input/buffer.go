package input

import (
	"sync"
)

// Buffer 玩家输入缓冲区
type Buffer struct {
	playerIndex int
	inputs      map[uint32][]byte // frame -> input
	maxFrames   int
	mu          sync.RWMutex
}

func NewBuffer(playerIndex, maxFrames int) *Buffer {
	return &Buffer{
		playerIndex: playerIndex,
		inputs:      make(map[uint32][]byte),
		maxFrames:   maxFrames,
	}
}

func (b *Buffer) Add(frame uint32, input []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 复制输入数据
	data := make([]byte, len(input))
	copy(data, input)
	b.inputs[frame] = data

	// 清理旧帧
	if len(b.inputs) > b.maxFrames {
		minFrame := uint32(0)
		for f := range b.inputs {
			if minFrame == 0 || f < minFrame {
				minFrame = f
			}
		}
		delete(b.inputs, minFrame)
	}
}

func (b *Buffer) Get(frame uint32) ([]byte, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	input, exists := b.inputs[frame]
	if !exists {
		return nil, false
	}
	return input, true
}

func (b *Buffer) GetRange(start, end uint32) map[uint32][]byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[uint32][]byte)
	for f, input := range b.inputs {
		if f >= start && f < end {
			result[f] = input
		}
	}
	return result
}

func (b *Buffer) Has(frame uint32) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, exists := b.inputs[frame]
	return exists
}

func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inputs = make(map[uint32][]byte)
}
