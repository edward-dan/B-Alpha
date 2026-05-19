# 系统总体拓扑结构

本文档定义系统的物理端、逻辑模块、状态流转和系统级生命周期动作，不包含任何具体策略公式。本文档与 `docs/strategy-math-engine.md`、`docs/evolution-engine.md` 共同构成本项目唯一功能真源。三份文档未定义的能力不得进入实现；发现需求越界时，先修改或确认文档，再写代码。

## 第 0 章：架构哲学

系统采用 SaaS、Agent、Lab 三端分工：SaaS 是决策与状态真源，Agent 是用户本地执行端，Lab 是离线计算端。三端通过明确的状态所有权和消息协议协作，避免任何一端偷偷承担不属于自己的职责。

所有业务推进必须被封装为离散动作。每次动作读取当前状态快照，产出确定性的下一步意图；动作之间不能依赖进程内隐式记忆、未持久化变量或“上一轮碰巧留下”的临时状态。

策略被视为纯计算契约，而不是服务。策略只接收 `StrategyInput` 并返回 `StrategyOutput`，不感知网络、数据库、文件系统、用户会话和 Agent 连接状态。这样才能保证回测与实盘共享同一个 `Step()`，并让复利、风控、进化计算在同一数学表面上被验证。

系统不做预防性解耦。当前阶段只保持 SaaS、Strategy、Agent 三层边界清楚，不为了未来不确定需求引入额外服务、事件总线或复杂插件框架。Redis 只缓存，不承担业务状态真源或跨端信号通道。

## 第 1 章：三端物理部署形态

### 1.1 SaaS 云端

SaaS 云端是系统的状态中心和策略执行中心。它负责用户认证、实例管理、策略参数包管理、调度 `RUNNING` 实例、构造 `StrategyInput`、调用 `Step()`、把 `StrategyOutput` 翻译为 `TradeCommand`，并通过 WebSocket 下发给对应 Agent。

SaaS 云端持有 Postgres 和 Redis 连接，但永不持有交易所 API Key。所有交易所私有接口调用都必须发生在 Agent 用户本地，SaaS 只能通过 Agent 上报的 `DeltaReport` 收敛真实资产状态。

SaaS 云端在 `app_role=saas` 下禁止创建和执行进化任务，禁止运行离线回测写接口。它可以读取冠军参数和历史报告，但不能把云端生产进程变成算力实验室。

### 1.2 Agent 用户本地

Agent 是用户本地运行的极简执行端。它读取 `config.agent.yaml` 中的 SaaS 连接信息和交易所 API 凭证，登录 SaaS 获取 JWT，建立 WebSocket 长连接，接收 `TradeCommand` 后调用交易所私有 REST API 执行订单。

Agent 不含策略代码，不连接数据库，不做自主交易决策。它收到命令后只负责确认接收、执行、查询成交、读取余额，并把结果包装为 `DeltaReport` 上报 SaaS。

Agent 断线后必须指数退避重连，重连成功后立即发送当前余额快照。Agent 不需要恢复本地策略状态，因为策略状态真源在 SaaS 侧持久化，端侧只提供可验证的执行结果和账户快照。

### 1.3 Lab 算力机

Lab 是本地或专用算力环境中的实验端，使用 `app_role=lab` 运行。它连接同一个 Postgres，但只开放 GA 进化、回测评估、挑战者参数包写入和基因库管理能力。

Lab 禁止下发真实交易指令，禁止启动或停止生产实例，禁止触发 Agent 执行。Lab 产出的结果只能以 `challenger` 角色写入数据库，等待人工审阅后 Promote 为 `champion`。

Lab 可以复用 SaaS 的模型、仓储、策略适配和回测适配，但不能绕过 `Step()` 直接调用策略内部函数。所有历史评估都必须通过与实盘同构的 `Step()` 契约执行。

## 第 2 章：app_role 三态行为矩阵

`app_role` 是进程级能力闸门，不是用户权限替代品。用户权限负责谁能操作，`app_role` 负责当前物理部署形态是否允许某类能力存在。路由层、服务层和调度入口都必须遵守该矩阵。

| app_role | 部署场景 | 开放能力 | 限制能力 | 禁区 |
|---|---|---|---|---|
| `saas` | 云端生产 SaaS | 用户认证、订阅校验、策略模板读取、实例创建/启动/停止、Cron Tick、`Step()` 执行、WebSocket 指令下发、Agent 上报处理、冠军参数读取 | 可读取进化结果和历史回测报告 | 禁止创建/执行 GA 进化任务；禁止回测写接口；禁止保存或接收 API Key |
| `lab` | 本地或专用算力机 | GA 进化任务、历史回测、挑战者参数包写入、基因库读取与管理、只读查看实例与冠军 | 可读取生产状态用于构造评估上下文 | 禁止真实交易指令下发；禁止实例启停；禁止 Agent 私有接口执行；禁止把 challenger 自动提升为 champion |
| `dev` | 本地开发测试 | SaaS、Lab、Agent 相关开发入口可全部启用 | 必须显式使用开发配置和测试凭证 | 禁止连接真实生产交易所密钥；禁止以 dev 配置操作生产数据库，除非人工明确授权 |

## 第 3 章：逻辑模块与职责边界

### 3.1 Strategy 策略模块

Strategy 模块定义策略接口、`Step()` 契约、参数结构和纯计算抽象。它是策略数学的产品中心，只描述如何从输入状态计算输出意图，不描述如何拉行情、如何存库、如何认证、如何下单。

策略包内部禁止网络、数据库、文件 I/O、定时器、随机全局状态和进程级缓存。若策略需要历史价格、资产状态、运行状态或参数包，必须由 SaaS 或回测适配层放入 `StrategyInput`。

Strategy 模块的禁区是执行与基础设施。它不得引用 `internal/saas`、`internal/agent`、GORM、Redis、HTTP 客户端、WebSocket 客户端或交易所 SDK。任何新增策略都必须先在 `docs/strategy-math-engine.md` 中有数学契约。

### 3.2 Instance 实例模块

Instance 模块管理“策略模板 + 标的 + 参数包 + 用户授权 + 资金语义”的运行实体。它负责实例创建、启动、停止、错误状态、运行状态快照、Portfolio State 读取与落账，以及 Cron Tick 对 `RUNNING` 实例的推进。

Instance 模块只把 `Step()` 的意图翻译为可执行 `TradeCommand`，不修改策略计算逻辑。它必须保证同一已完成聚合桶只执行一次 `Step()`，避免实盘比回测多跑或少跑决策。

Instance 模块的禁区是交易所私钥和自主策略。它不能持有 API Key，不能直接调用交易所私有下单接口，不能在 `Step()` 外另写风控策略或额外买卖规则。执行结果只能来自 Agent 的 `DeltaReport`。

### 3.3 Evolution 进化模块

Evolution 模块是参数实验室，负责遗传算法种群生命周期、并发适应度评估、多窗口坩埚评分、挑战者结果写入和 Promote 所需的数据准备。它不关心具体策略染色体内部字段，只通过 `EvolvableStrategy` 接口操作不透明基因。

Evolution 模块可以调用回测适配层，但回测适配层也必须调用与实盘相同的 `Step()`。进化结果默认是 `challenger`，不得自动影响生产实例；只有人工 Promote 后才成为 `champion`。

Evolution 模块的禁区是真实交易和具体策略内核。它不能 import 交易所执行器，不能下发 `TradeCommand`，不能读取 Agent API Key，不能为了性能绕过策略契约写一套独立回测逻辑。

### 3.4 Auth 认证模块

Auth 模块负责用户注册、登录、JWT 签发、Agent 登录、订阅计划与配额校验。它为 SaaS API 和 WebSocket 握手提供身份边界，但不参与策略决策。

用户订阅影响实例数量、可用模板、算力任务额度和 API 调用权限，不影响 `Step()` 内部数学结果。相同输入和相同参数包进入 `Step()` 时，不能因为用户等级不同而改变策略输出。

Auth 模块的禁区是交易和策略。它不能保存交易所私钥，不能构造交易指令，不能修改 Portfolio State，也不能把认证状态写入策略运行状态。

## 第 4 章：全局状态总线

系统使用单一 Postgres 作为持久化状态真源，使用 Redis 作为缓存和短期会话辅助。Redis 不承载必须持久化的业务事实，不作为跨模块指令通道，也不作为交易状态确认机制。

Postgres 中的数据必须有明确所有权：谁创建、谁更新、谁读取、谁不能写。跨端状态流转只能通过数据库事务和 WebSocket 协议完成，不能通过共享文件、本地临时缓存或 Redis 发布订阅绕过。

SaaS 收到 Agent 上报后更新状态，下一次 `Step()` 基于最新持久化状态重新计算。系统不追求端侧和云端每毫秒一致，而追求每个完成动作之后都能收敛到可审计的最终状态。

| 数据类别 | 真源位置 | 写入方 | 读取方 | 说明 |
|---|---|---|---|---|
| 用户、订阅、权限 | Postgres | SaaS Auth | SaaS API、调度入口 | 用户身份与配额统一由 SaaS 管理 |
| 策略模板元数据 | Postgres + 代码版本 | SaaS 管理流程 | SaaS、Lab | 模板必须对应代码中的 `StrategyID` 和版本 |
| `Step()` 数学契约 | `docs/strategy-math-engine.md` + 策略代码 | 文档与代码变更流程 | SaaS、Lab 回测适配 | 回测与实盘唯一入口 |
| 冠军参数包 `champion` | Postgres，Redis 可缓存 | Promote 事务 | SaaS Tick、实例创建 | Redis 命中仅是缓存，DB 是真源 |
| 挑战者参数包 `challenger` | Postgres | Lab Evolution | Lab、SaaS 只读、人工审批 | 不自动进入生产 |
| 历史基因 `retired` | Postgres | Promote 事务 | Lab、审计 | 只读存档 |
| 运行中实例状态 | Postgres | SaaS Instance | SaaS Tick、前端 | `RUNNING`/`STOPPED`/`ERROR` 等状态 |
| Portfolio State | Postgres | SaaS 根据 `DeltaReport` 和语义转换更新 | SaaS `Step()`、Lab 回放 | 真实余额上报 + SaaS 语义账本收敛 |
| Runtime State | Postgres | SaaS `Step()` 后持久化 | SaaS `Step()`、回测适配 | 策略运行状态快照，不由 Agent 写入 |
| 交易指令 pending 记录 | Postgres | SaaS 下发前写入 | SaaS、审计 | 用 `client_order_id` 去重和回填 |
| 成交明细与余额快照 | Postgres | SaaS 处理 Agent 上报 | SaaS、前端、审计 | Agent 是事实来源，SaaS 是持久化真源 |
| Agent 连接状态 | SaaS 内存 + Redis 可辅助缓存 | WebSocket Hub | SaaS Tick | 连接状态可丢失，重连后自愈 |
| 交易所 API Key | Agent 本地 `config.agent.yaml` | 用户本地 | Agent | 永不进入 SaaS、DB、日志和网络上行 |

## 第 5 章：WebSocket 通信协议

WebSocket 协议遵循“云端决策、端侧执行、事实上报、状态收敛”。SaaS 不假设命令一定成功，Agent 不假设自己能决定下一步。所有消息必须携带可审计的实例、时间和幂等标识。

Agent 主动连接 SaaS `/ws/agent`。连接建立后第一条消息必须是 `auth`，鉴权通过前 SaaS 不发送命令。心跳用于连接存活检测，不代表资产状态正确；资产状态只能由 `delta_report` 收敛。

### 5.1 消息类型全表

| 消息类型 | 方向 | 触发时机 | 幂等与状态语义 |
|---|---|---|---|
| `auth` | Agent -> SaaS | WebSocket 建立后立即发送 | 携带 JWT、agent_id、版本号；鉴权失败立即关闭连接 |
| `auth_result` | SaaS -> Agent | SaaS 校验 `auth` 后返回 | 成功后连接进入已认证状态；失败时包含错误码 |
| `heartbeat` | Agent -> SaaS | 默认每 30 秒发送 | 只表示连接存活，可附带本地时间和版本 |
| `heartbeat_ack` | SaaS -> Agent | SaaS 收到心跳后返回 | 用于 Agent 判断链路延迟，不更新资产 |
| `command` | SaaS -> Agent | `Step()` 产出交易意图并落 pending 记录后 | 以 `client_order_id` 幂等，Agent 重复收到不得重复下单 |
| `command_ack` | Agent -> SaaS | Agent 收到且完成本地幂等检查后立即发送 | 只确认收到，不代表成交成功 |
| `delta_report` | Agent -> SaaS | 命令执行完成、失败终态、重连初始快照、定期对账快照 | 资产事实来源；可无 `client_order_id` 表示纯快照 |
| `report_ack` | SaaS -> Agent | SaaS 成功处理并持久化 `delta_report` 后 | Agent 可清理本地待确认上报 |

### 5.2 TradeCommand 字段语义

`TradeCommand` 是 SaaS 下发给 Agent 的唯一交易指令形态。SaaS 必须先写 pending 记录再下发，Agent 必须按 `client_order_id` 做本地幂等保护。

| 字段 | 类型 | 语义 |
|---|---|---|
| `client_order_id` | string | 全局唯一幂等键，建议格式 `inst{instance_id}-{engine}-{bucket_ts}-{nonce}` |
| `instance_id` | string/int | SaaS 实例标识，Agent 用于路由本地账户配置 |
| `symbol` | string | 交易对，如 `BTCUSDT` |
| `action` | enum | `BUY` 或 `SELL`；现货语义下买入花费 USDT，卖出释放 BTC |
| `engine` | enum | `MACRO` 或 `MICRO`，表示意图来源 |
| `lot_type` | enum | `DEAD_STACK` 或 `FLOATING`；宏观吸入底仓或微观浮动仓 |
| `amount_usdt` | decimal string | 买入时使用，表示最多花费的 USDT 数量 |
| `qty_asset` | decimal string | 卖出时使用，表示最多卖出的标的资产数量 |
| `limit_price` | decimal string/null | Phase 1 仅定义字段语义；为空表示市价或交易所侧默认执行方式 |
| `created_at_ms` | int64 | SaaS 创建命令时间 |
| `expires_at_ms` | int64 | 命令过期时间，Agent 过期后不得执行 |
| `reason_code` | string | 策略输出的机器可读原因，用于审计，不给用户暴露裸数学 |

### 5.3 DeltaReport 字段语义

`DeltaReport` 是 Agent 向 SaaS 上报执行事实和余额事实的唯一协议。SaaS 只用它更新执行记录、资产快照和 Portfolio State，不接受 Agent 上传策略决策。

| 字段 | 类型 | 语义 |
|---|---|---|
| `report_id` | string | Agent 生成的上报幂等键，SaaS 重复收到必须安全忽略 |
| `client_order_id` | string/null | 对应 `TradeCommand`；重连快照或定期对账可为空 |
| `instance_id` | string/int | SaaS 实例标识 |
| `symbol` | string | 交易对 |
| `status` | enum | `FILLED`、`PARTIALLY_FILLED`、`REJECTED`、`EXPIRED`、`FAILED`、`SNAPSHOT` |
| `execution` | object/null | 成交均价、成交数量、成交金额、手续费、交易所订单号、成交时间 |
| `balances` | array/object | 当前资产余额快照，至少包含标的资产和 USDT 的 available、locked |
| `exchange_time_ms` | int64/null | 交易所返回的成交或余额时间 |
| `agent_time_ms` | int64 | Agent 本地上报时间 |
| `error_code` | string/null | 执行失败时的机器可读错误码 |
| `error_message` | string/null | 执行失败摘要，禁止包含 API Key 或 Secret |

### 5.4 状态收敛与天然自愈机制

SaaS 收到 `delta_report` 后，在事务中完成去重、pending 执行记录回填、成交明细持久化、余额快照持久化和 Portfolio State 更新。`report_ack` 只在事务成功后发送。

如果 Agent 断线，SaaS 对该实例跳过下发并记录可审计事件，不制造本地虚假成交。Agent 重连后先上报余额快照，SaaS 以真实余额修正 Portfolio State，下一次 Cron Tick 自然基于新状态重新决策。

如果 SaaS 重启，它从 Postgres 恢复实例、pending 命令、Runtime State 和 Portfolio State。未确认命令不会被假设成功；只有后续 `delta_report` 或人工对账能改变执行终态。

## 第 6 章：系统级生命周期动作

### 6.1 系统初始化流程

SaaS 启动时读取配置，初始化日志、Postgres、Redis、JWT、WebSocket Hub 和 Cron 调度器。数据库结构以 Go struct 为 schema 真源，通过 GORM `AutoMigrate` 同步，不使用 SQL migration 文件。

初始化阶段从 DB 加载策略模板元数据、冠军参数缓存、`RUNNING` 实例列表和必要的 Runtime State。任何无法解析的关键状态必须让实例进入 `ERROR` 或阻止进程启动，不能静默使用零值推进交易。

初始化完成后，SaaS 开始接收 Agent 连接和 API 请求。Cron 可以扫描实例，但每个实例是否真正执行 `Step()` 取决于最新已完成聚合桶是否已经处理过。

### 6.2 Cron Tick 驱动 Step() 的完整流程

Cron 以基础扫描频率枚举 `RUNNING` 实例。对每个实例，SaaS 先判断其策略周期对应的最新已完成聚合桶；若该桶已处理，则跳过，保证实盘不会比回测多执行。

需要推进时，SaaS 读取公开行情、当前 Portfolio State、Runtime State、冠军参数包、实例配置和交易精度，构造 `StrategyInput`。随后在 SaaS 进程内调用唯一入口 `Step(StrategyInput) -> StrategyOutput`。

`StrategyOutput` 可能包含交易意图、语义账本转换、下一份 Runtime State 和审计原因。SaaS 先持久化 Runtime State 和账本内转换，再把可执行交易意图翻译为 `TradeCommand`，写入 pending 记录后通过 WebSocket 下发给对应 Agent。

如果 Agent 未连接、命令过期、余额不足或交易精度不满足，SaaS 不得伪造成交。该 tick 的处理结果必须进入审计日志，后续 tick 会基于最新上报状态重新收敛。

### 6.3 Agent 断线重连指数退避

Agent 检测到 WebSocket 断开后，立即停止接收新命令，保留本地未确认上报队列，并按指数退避重新登录和连接。退避从 1 秒开始，每次失败翻倍，最大 5 分钟，成功后重置。

重连成功后，Agent 先发送 `auth`，通过后立即发送 `delta_report` 快照，包含当前交易所余额，不包含策略意图。若本地存在已执行但未收到 `report_ack` 的报告，必须按原 `report_id` 重发，依赖 SaaS 幂等处理。

SaaS 在 Agent 断线期间不向该 Agent 下发新命令。断线恢复不需要人工修复策略状态，因为真实余额快照和 pending 记录会在下一次 tick 前后自然收敛。

### 6.4 优雅停机与状态快照

SaaS 收到 `SIGTERM` 或等效关闭信号后，先停止接收新的 API 写请求、Cron Tick 和 Agent 命令下发。正在执行的 tick 在超时保护内完成；未开始的 tick 直接取消。

停机流程必须持久化已完成 `Step()` 的 Runtime State、账本转换和 pending 命令。不能把已经下发但未确认的命令标记为成功，也不能丢弃已收到但未处理完成的 `delta_report`。

Agent 停机时应停止读取新命令，尽量完成已接收命令的最终上报，并关闭 WebSocket。若无法完成，上报幂等键和交易所订单号必须保留在本地恢复路径中，重启后继续上报。

## 第 7 章：不可推翻的技术决策

以下决策是系统铁律。任何实现、重构、测试或文档变更若违反本章约束，必须停止并回到文档确认。

1. 策略必须满足复利前置条件。资金规模、目标仓位或订单规模必须能随权益变化而滚动，不能只是一组固定金额脚本。
2. 回测与实盘必须调用同一个 `Step()` 实现。`Step()` 内部禁止 `isBacktest`、`isLive` 等环境分叉。
3. `Step()` 只在 SaaS 侧执行。Agent 二进制不包含策略计算代码，Lab 回测也通过 SaaS 同构适配调用 `Step()`。
4. 策略包内部禁止网络、数据库、文件 I/O、定时器和交易所 SDK。
5. API Key 只能存在于 Agent 本地 `config.agent.yaml`，永不进入 SaaS、Postgres、Redis、日志、错误上报或 WebSocket 上行消息。
6. 数据库结构遵守 GORM Code-First，只使用 `AutoMigrate` 管理模型结构，不维护 SQL migration 文件。
7. 价格、收益、止损、止盈、仓位计算优先使用无量纲表达，如比例、权重、对数收益率、标准化偏离和回撤。
8. 系统只使用单一 Postgres 作为持久化状态总线；Redis 仅做缓存和会话辅助，不做业务信号真源。
9. SaaS、Strategy、Agent 三层职责不可混用。SaaS 决策与持久化，Strategy 纯计算，Agent 执行与上报。
10. 回测适配层不得另写策略逻辑，只负责把历史数据和状态喂给同一个 `Step()`。
11. `DeadBTC`、`FloatBTC`、`ColdSealedBTC` 三态仓位语义不能混账。任何账本转换必须由 `Step()` 输出并由 SaaS 审计落账。
12. `DeadBTC -> FloatBTC` 的释放只改变 SaaS 侧语义账本，不直接下发 Agent 命令；真实卖出仍必须由后续微观引擎意图产生。
13. 面向用户的界面文案不得暴露无上下文的内部公式、裸希腊字母或状态机黑话；内部原因码可以保留给审计和诊断。
