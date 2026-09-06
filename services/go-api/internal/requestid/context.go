// Package requestid 在 HTTP 与内部调用之间透传请求标识。
package requestid

import "context"

type contextKey struct{}

// WithContext 将 request ID 写入标准 context，供 gRPC 等下游调用读取。
func WithContext(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, contextKey{}, value)
}

// FromContext 返回当前请求标识；缺失时返回空字符串。
func FromContext(ctx context.Context) string {
	value, _ := ctx.Value(contextKey{}).(string)
	return value
}
