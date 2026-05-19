# 项目 AI 工作约束

本文件是 Codex、Claude、Cursor 等 AI 协作工具处理本项目时必须遵守的统一规范。所有实现、重构、测试和文档修改都必须先满足本文件约束。

## 1. 唯一功能真源

当前项目功能只依据 `docs/` 下的三份文档：

- `docs/system-topology.md`
- `docs/strategy-math-engine.md`
- `docs/evolution-engine.md`

三份文档没有定义的功能，不进入实现。发现需求超出文档范围时，先补充或确认文档，再进入代码实现。

## 2. 工作顺序

- 涉及策略和回测时，必须先阅读 `docs/strategy-math-engine.md` 和 `docs/evolution-engine.md`。
- 涉及 Go 后端时，遵守 GORM Code-First，只使用 `AutoMigrate` 管理模型结构。
- 涉及价格、收益、止损、止盈、仓位计算时，优先使用无量纲表达。
- 涉及架构边界时，保持 SaaS、Strategy、Agent 三层分工，不做预防性解耦。

## 3. 核心约束

- 策略必须满足复利前置条件。
- 回测与实盘必须调用同一个 `Step()` 实现。
- `Step()` 只在 SaaS 侧执行。
- 策略包内部禁止网络、数据库、文件 I/O。
- API Key 只能存在于 `config.agent.yaml`。

## 4. 代码目录职责

- `cmd/saas/`: SaaS 进程入口，只负责启动 SaaS 服务、调度和依赖装配。
- `cmd/agent/`: Agent 进程入口，只负责启动 Agent 本地运行时和心跳。
- `internal/saas/`: SaaS 侧业务边界，包含 API、配置、模型、服务、仓储和调度。
- `internal/agent/`: Agent 侧业务边界，包含本地配置、交易所适配、执行器、行情采集和心跳。
- `internal/strategy/`: 策略接口、`Step()` 契约和策略运行所需的纯计算抽象；禁止 I/O。
- `internal/strategies/[策略名]/`: 单个策略实现目录，只放该策略的纯计算逻辑和参数定义。
- `internal/quant/`: 通用量化计算模块，包含指标、风险、绩效和仓位计算。
- `internal/adapters/backtest/`: 回测适配层，只负责把历史数据接入 SaaS 侧同一个 `Step()`，不得另写策略逻辑。

## 5. 验证命令

每次修改后至少执行：

```bash
go list ./...
go test ./...
```

## 6. Phase 0 禁止事项

Phase 0 只做项目地基，不进入业务实现。本阶段禁止实现：

- 具体策略逻辑
- 回测逻辑
- 实盘交易逻辑
- 交易所连接逻辑
- 数据库业务模型
- SaaS API
- Agent 任务执行逻辑
- 参数优化逻辑
