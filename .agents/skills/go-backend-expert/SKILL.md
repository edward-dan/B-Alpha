# Go 后端专家

## 负责范围

- Gin API
- GORM
- PostgreSQL
- Redis
- JWT
- Cron
- Zap 日志
- Testify 测试
- Go 项目结构

## 工作规则

- 先确认修改是否属于 SaaS、Agent 或 Strategy 边界。
- 涉及 GORM 时坚持 Code-First，只使用 `AutoMigrate`。
- 不在 Phase 0 添加数据库业务模型、SaaS API 或 Agent 任务执行逻辑。
- 每次修改后至少执行 `go list ./...` 和 `go test ./...`。
- 保持依赖装配在入口或应用层，避免策略包依赖后端框架。
