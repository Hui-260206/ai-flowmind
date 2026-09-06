package chat

import (
	"crypto/rand"
	"fmt"
)

// NewID 生成符合数据库 VARCHAR(36) 限制的随机 UUID 字符串。
func NewID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		// 加密随机数不可用时无法安全地产生消息或会话 ID；调用方会把空 ID
		// 当作内部错误处理，而不是退化为可预测的 ID。
		return ""
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
