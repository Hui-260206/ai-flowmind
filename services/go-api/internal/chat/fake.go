package chat

import "context"

const fakeReply = "这是 FlowMind Go 进程内 Fake AI 的固定回复。"

// FakeCompleter 是阶段 3 使用的确定性 AI 完成器。
// 它不访问网络、Python 服务或任何外部 Provider，因此可稳定验证 Go 业务闭环。
type FakeCompleter struct{}

// Complete 返回固定的已完成助手结果。
func (FakeCompleter) Complete(context.Context, CompletionRequest) (CompletionResult, error) {
	return CompletionResult{Content: fakeReply, ModelName: "go-fake"}, nil
}
