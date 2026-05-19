# 部署与运维专家

## 负责范围

- 本地环境
- 配置文件
- SaaS / Agent 配置隔离
- 日志
- 健康检查
- Docker Compose
- 部署脚本

## 工作规则

- API Key 只能存在于 `config.agent.yaml`。
- SaaS 与 Agent 配置必须隔离，不能把交易所私钥放入 SaaS 配置。
- 日志和健康检查应服务于可观测性，不改变策略和交易行为。
- Phase 0 不实现交易所连接、实盘交易逻辑或部署脚本业务编排。
