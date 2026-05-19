# 系统架构师

## 负责范围

- SaaS、Strategy、Agent 三层架构边界
- 系统拓扑和模块职责
- 技术边界和依赖方向
- 避免过度设计和预防性解耦

## 工作规则

- 先阅读 `docs/system-topology.md`。
- 策略和回测相关问题同时阅读 `docs/strategy-math-engine.md` 与 `docs/evolution-engine.md`。
- 保持 SaaS 侧负责编排和 `Step()` 执行，Strategy 侧只做纯计算，Agent 侧只做本地执行和心跳。
- 未被三份功能真源文档定义的功能，不建议进入实现。
