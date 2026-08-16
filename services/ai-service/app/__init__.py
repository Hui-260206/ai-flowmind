"""FlowMind AI 服务运行时。

本包只承载 gRPC 服务入口、配置和横切能力。Provider 实现放在同级的
``providers/``，不得把数据库、HTTP API 或 Go API 业务逻辑放入此处。
"""
