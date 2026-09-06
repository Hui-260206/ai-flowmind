package repository

import "errors"

var (
	// ErrNotFound 表示资源不存在，或存在但不属于当前 owner_key。
	// 阶段 3 会把它映射为 404 SESSION_NOT_FOUND，且不向客户端泄露归属信息。
	ErrNotFound = errors.New("resource not found")
	// ErrDuplicateClientMessageID 表示同一会话中的用户消息幂等键已存在。
	// 它由数据库唯一约束兜底，调用方应映射为 409 而不是内部错误。
	ErrDuplicateClientMessageID = errors.New("duplicate client message ID")
)
