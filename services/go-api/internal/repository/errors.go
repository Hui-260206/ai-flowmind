package repository

import "errors"

// ErrNotFound 表示资源不存在，或存在但不属于当前 owner_key。
// 阶段 3 会把它映射为 404 SESSION_NOT_FOUND，且不向客户端泄露归属信息。
var ErrNotFound = errors.New("resource not found")
