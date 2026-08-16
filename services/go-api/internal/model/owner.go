package model

// OwnerKey 把匿名 client_id 派生为会话/消息的归属键。
//
// 这是全项目唯一允许构造 owner_key 的地方；未来接入登录时只需把这里改成
// "user:" + userID，其余所有查询条件的写法保持不变。
func OwnerKey(clientID string) string {
	return "client:" + clientID
}
