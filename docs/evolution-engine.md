# 进化计算引擎

本文档定义 GA 遗传算法引擎的完整规格。进化引擎是一个纯计算黑盒，不关心具体策略内部结构；它只通过抽象接口驱动种群生命周期，并把结果交付为等待人工审批的 `challenger`。

## 第 0 章：核心定位

进化引擎是纯计算线程。它接收策略提供的基因空间、出生点、历史数据窗口和进化配置，对不透明基因执行采样、评估、选择、交叉、变异、缓存和收敛检测。

进化引擎不 import 任何具体策略实现，不读取染色体字段名，不理解宏观引擎、微观引擎或仓位三态。具体策略通过 `EvolvableStrategy` 接口把基因操作和适应度评估封装起来。

引擎输入是：`EvolvableStrategy`、`EvaluablePlan`、历史 K 线窗口、出生点快照、种群规模、最大代数、随机种子和运行上下文。引擎输出是：最优基因、评分明细、窗口报告和可写入 DB 的参数包。

引擎不做真实交易，不下发 `TradeCommand`，不连接 Agent，不读取交易所 API Key。所有评估必须通过回测适配层调用与实盘同一个 `Step()`。

## 第 1 章：探索窗口定义

### 1.1 多时段坩埚

每个 Epoch 启动时，引擎构造四个评估窗口：全量历史、5 年、2 年、6 个月。窗口权重固定为 `0.40 / 0.30 / 0.20 / 0.10`，用于衡量策略在长周期、近周期和短周期中的稳健性。

| 窗口 | 评分区间 | 权重 | 说明 |
|---|---|---:|---|
| `full` | 从入库最早可用 closed bar 到最新 closed bar | 0.40 | 不设人工十年上限；覆盖尽可能长的市场阶段 |
| `5y` | 最新 closed bar 往前 1825 天 | 0.30 | 中长期主窗口 |
| `2y` | 最新 closed bar 往前 730 天 | 0.20 | 近周期适应性窗口 |
| `6m` | 最新 closed bar 往前 183 天 | 0.10 | 短周期压力窗口 |

5 年、2 年、6 个月窗口可以在评分区间之前附带 warmup bars，用于指标预热；warmup 不计入评分。全量窗口以全序列起点为评估起点，若指标需要预热，预热期收益不纳入窗口评分。

严禁未来数据泄露。任意时间点的 `Step()` 只能看到该时间点及之前已经完全闭合的 bars。窗口切片、指标预热、DCA 基线和策略评估都不得读取评分时点之后的数据。

### 1.2 基因空间的三个语义操作

策略必须为基因空间提供三个语义操作，即使它们被封装在 `Sample`、`Mutate`、`Crossover` 内部也必须存在清晰职责。

`Sample` 从合法空间随机生成一个基因实例。采样结果必须满足字段边界和结构约束，不能依赖后续评估时再修复。

`Clamp` 把越界或结构损坏的基因修复回合法空间。它处理数值边界、枚举边界、EMA 顺序、窗口相对关系和权重上下界。

`Validate` 检查基因是否合法。无法修复或修复后仍违反硬约束的基因不得进入正常评估，应被拒绝或返回 fatal 评分。

### 1.3 出生点冻结

出生点是 Epoch 级共享上下文，包含初始资金、月度注资、标的、交易精度、手续费假设、风险边界和历史窗口。它描述本次进化的“出生环境”，不属于染色体。

一个 Epoch 内，所有个体共享同一个出生点。出生点不参与代内交叉、变异和指纹缓存；若需要比较不同出生点，应创建新的进化任务或新的 Epoch。

`spawn_mode` 支持三种语义：`inherit` 表示继承当前 champion 或产品默认出生点；`random_once` 表示任务排队时随机采样一次并冻结；`manual` 表示请求体显式提交出生点。无论哪种模式，Epoch 开始后都不可在代内改变。

## 第 2 章：进化生命周期动作

### 2.1 种群初始化策略

种群初始化以当前种子冠军为锚点。`index 0` 始终为当前种子冠军原样，不做变异，不做交叉，不被随机个体覆盖，用于保证进化不会丢失已知基准。

当数据库存在可用精英时，除 `index 0` 外的剩余个体按比例生成：10% 精英继承，40% 基于精英强化变异，50% 完全随机。强化变异使用较高变异尺度探索冠军附近邻域，但仍需 Clamp 和 Validate。

当数据库没有可用精英时，`index 0` 使用产品默认冠军，其余个体全部随机采样。初始化阶段不得使用未来窗口评分结果来筛选个体。

### 2.2 并发适应度评估

适应度评估使用固定大小 worker pool：

```text
Workers = min(NumCPU, PopulationSize)
```

每个 worker 独占回测适配上下文，不共享可变策略状态。共享的只读数据包括 `EvaluablePlan`、窗口 bars 和预计算 DCA 基线。

所有个体评估完成后才能进入选择和繁殖阶段。若上下文取消、任务暂停或系统停机，当前代应返回可审计的中断状态，不写入伪完成结果。

### 2.3 锦标赛选择

父代选择使用锦标赛选择，`TournamentSize=3`。每次随机抽取 3 个个体，取适应度最高者作为父代。

fatal 个体不需要显式过滤，因为其评分极低，在锦标赛中自然失败。随机抽样必须只在当前代种群内进行，不能把下一代新生个体提前放入候选池。

锦标赛选择保留足够选择压力，同时避免单一冠军过早垄断基因池。随机源应由 Epoch 级 seed 控制，便于复现实验。

### 2.4 均匀交叉

交叉使用 Uniform Crossover。对每个基因维度，以 50% 概率从父代 A 取值，以 50% 概率从父代 B 取值。

引擎不理解基因维度语义，因此交叉后的结构修复必须交给策略实现。策略的 `Crossover` 应在拼接后执行 Clamp，并通过 Validate 确认 EMA 顺序、窗口比例、权重边界等结构约束。

若基因内部包含强耦合字段，策略可以在 `Crossover` 内把这些字段作为块处理，但对引擎暴露的仍是一个不透明 `Gene`。

### 2.5 加性高斯变异

变异使用每维独立 Bernoulli 决策。某一维是否变异由 `MutationProbability` 控制，变异量来自正态分布：

```text
gene[i] = gene[i] + Normal(0, 1) * GeneStep[i] * MutationScale
```

枚举、整数和周期类字段由策略实现负责离散化、取整和结构修复。变异后必须 Clamp，无法合法化的基因必须 Validate 失败。

默认 `MutationProbability=0.15`，`MutationScale=1.0`。初始化强化变异可使用独立配置，例如概率 `0.15`、尺度 `1.5`，但不影响代内 Mutation Ramp 的当前值。

### 2.6 精英保留

每代将适应度 Top N 个体直接复制到下一代，默认 `Elitism=8`。精英保留不交叉、不变异、不重新采样，但可以通过 Fingerprint Cache 复用评分。

精英保留数量必须小于种群规模，并且不能挤占 `index 0` 种子冠军的语义。若 `index 0` 在某代不属于 Top N，它仍可作为普通个体参与评分，但下一代保留遵循统一排序结果。

精英保留的目的是防止随机繁殖破坏已发现的优解，不是保证旧冠军永远胜出。最终交付以最高适应度个体为准。

### 2.7 收敛检测与变异斜坡（Mutation Ramp）

引擎跟踪每代最佳适应度。当连续 `EarlyStopPatience` 代改善低于 `EarlyStopMinDelta` 时，先触发变异斜坡，而不是立即停止。

```text
MutationProbability *= MutationRampProbFactor
MutationScale *= MutationRampScaleFactor
```

默认 `EarlyStopPatience=5`，`EarlyStopMinDelta=0.001`，`MutationRampProbFactor=1.25`，`MutationRampScaleFactor=1.25`。上限为 `MutationProbabilityMax=0.55`、`MutationScaleMax=3.0`。

只有当变异概率和幅度都触及上限后仍连续无改善，才允许 Early Stop。Early Stop 必须记录停止代数、最佳评分、当前 ramp 参数和无改善计数。

### 2.8 基因组指纹缓存

每个基因由策略提供 Fingerprint，使用 FNV-1a-64 哈希，浮点字段量化精度为 `1e-6`。指纹必须只覆盖染色体，不覆盖出生点。

Epoch 内维护 Fingerprint Cache。若新个体指纹命中缓存，引擎直接复用已评估结果，跳过重复回测。缓存命中仍需在个体报告中标记，便于分析重复率。

指纹不负责判断基因优劣，只负责重复消除。不同出生点或不同历史窗口下不得复用旧 Epoch 的评分缓存，除非未来文档明确引入跨 Epoch 缓存协议。

## 第 3 章：适应度黑盒（Fitness Function）

### 3.1 多窗口坩埚分数公式

每个窗口都把策略表现与被动 Ghost DCA 基准比较。窗口内先计算策略 ROI、策略最大回撤、Ghost DCA ROI 和 Ghost DCA 最大回撤。

```text
Alpha = ROI_strategy - ROI_passive_DCA

SliceScore =
  Alpha - 1.5 * max(0, MaxDD_strategy - MaxDD_passive_DCA)
```

该公式奖励跑赢被动 DCA 的超额收益，同时惩罚超过被动 DCA 的额外回撤。回撤惩罚只惩罚超额部分，不因市场自身下跌重复惩罚策略。

### 3.2 Fatal 判断

当任意窗口内 `MaxDD_strategy >= 88%` 时，该窗口 `SliceScore = -99999`，并触发 hard fatal。fatal 是硬否决，不允许通过其他窗口高分抵消。

fatal 代表策略路径已经出现接近不可恢复的资本损毁。触发 fatal 后评估应立即返回 fatal 结果，并记录触发窗口、触发时间和最大回撤。

fatal 个体仍可留在当前代报告中，但不会成为有效冠军。锦标赛选择中 fatal 因分数极低自然淘汰。

### 3.3 加权汇总

窗口评分按固定权重汇总：

```text
ScoreTotal =
  0.40 * Score_full
  + 0.30 * Score_5y
  + 0.20 * Score_2y
  + 0.10 * Score_6m
```

权重不可由 HTTP 请求临时覆盖，避免为了某次回测结果调整评价法律。若未来要改变窗口权重，必须先修改本文档并重新审视历史报告可比性。

缺失窗口不能静默按 0 分处理。若历史数据不足以构造某窗口，应在任务创建或 EvaluablePlan 构建阶段失败，或按文档新增的数据不足策略处理。

### 3.4 Ghost DCA 基准

Ghost DCA 是被动对照组：起始时用种子资本买入标的资产，之后每个自然月月初注资并把可用 USDT 全部买入标的。它不使用策略信号，不做卖出，不做参数寻优。

Ghost DCA 使用与策略评估相同的价格序列、手续费假设、交易精度和注资计划。它的作用是回答一个核心问题：该策略是否值得比“无脑长期定投”更复杂。

每个窗口的 DCA 基线在 Epoch 启动时预计算，并放入 `EvaluablePlan`。代内所有个体共享同一基线，不重复计算。

### 3.5 Modified Dietz 收益率

ROI 使用 Modified Dietz 思路，剔除外部注资导致的 NAV 跳变。收益率不把“用户又注入了现金”误算成策略收益。

简化语义为：窗口收益 = 期末权益减期初权益和外部净流入后的投资收益，除以按资金在场时间加权后的资本基数。具体实现必须保证月初注资不会立即抬高 ROI。

最大回撤基于净资产曲线计算。净资产曲线应在注资点做资本流修正，使回撤反映市场和策略损失，而不是现金流时间造成的假跌幅。

### 3.6 级联短路

评估按短到长顺序执行：

```text
6m -> 2y -> 5y -> full
```

任一窗口触发 fatal 时立即退出，跳过后续更长窗口。短路降低早期随机种群中的无效计算，尤其在 fatal 个体比例较高时节省大量回测时间。

若所有窗口均未 fatal，则按第 3.3 节加权汇总。报告中必须保留每个窗口的评分、ROI、MaxDD、Alpha、DCA 对照和是否命中缓存。

## 第 4 章：EvolvableStrategy 接口（8-verb 契约）

进化引擎通过 8 个动词与具体策略交互。`Gene` 对引擎是不透明值，类型可以是结构体、JSON、字节数组或其他策略内部表示。

| 动词 | 职责 |
|---|---|
| `StrategyID()` | 返回策略唯一标识和版本兼容信息，用于任务、参数包和报告归属 |
| `Sample(rng)` | 从合法基因空间随机采样一个 `Gene`，内部必须 Clamp 和 Validate |
| `Mutate(gene, prob, scale, rng)` | 对 `Gene` 执行加性高斯或策略自定义等价变异，返回合法子代 |
| `Crossover(parentA, parentB, rng)` | 执行均匀交叉或策略等价交叉，返回合法子代 |
| `Fingerprint(gene)` | 返回 FNV-1a-64 指纹，浮点量化精度 `1e-6`，只覆盖染色体 |
| `Evaluate(ctx, gene, plan)` | 在只读 `EvaluablePlan` 上回测并返回总分、窗口明细和 fatal 状态 |
| `DecodeElite(paramPackJSON)` | 从 DB 参数包解码历史精英；空值时返回产品默认种子 |
| `EncodeResult(gene, spawnPoint)` | 将获胜基因和出生点编码为可入库 ParamPack JSON |

引擎不得通过类型断言、反射字段名或 JSON path 读取染色体内部字段。新增策略只需实现该接口，不应修改引擎主循环。

Clamp 和 Validate 虽不单列为 8 个公共动词，但必须由 `Sample`、`Mutate`、`Crossover` 在策略内部执行。`Evaluate` 可以对输入基因再次 Validate，防止历史坏数据进入回测。

## 第 5 章：EvaluablePlan 只读上下文

`EvaluablePlan` 在 Epoch 启动时构建，整个世代内不可变。所有 worker 共享它的只读引用，不允许在个体评估过程中修改计划内容。

`EvaluablePlan` 至少包含：标的信息、策略模板标识、出生点快照、四个坩埚窗口、每个窗口的 warmup 和评分区间、交易精度、手续费假设、预计算 Ghost DCA 基线、随机种子和风险硬阈值。

示意结构如下：

```text
EvaluablePlan
  Pair
  TemplateName
  SpawnPoint
  LotStep
  LotMin
  FeeModel
  Windows[]CrucibleWindow
  DCABaselines[]DCABaseline
  RiskBounds
  DataCoverage
```

`DataCoverage` 必须记录历史数据起止时间、最新 closed bar 时间、每个窗口评分起点和 warmup 起点。任何回测报告都应能说明评分使用了哪段数据。

构建 `EvaluablePlan` 前必须完成公共行情覆盖检查。若 `market_klines` 中缺少目标交易对、周期或窗口所需 closed bars，Lab 可以先通过 Binance 正式公开历史 K 线 API 补齐缺口；补齐完成后仍无法满足窗口覆盖要求时，进化任务或回测任务必须失败，不能用残缺窗口计算适应度。

多币种回测或多币种进化评估必须在计划中记录每个交易对的数据覆盖、周期、起止时间和缺口补齐结果。共享虚拟账户的回测报告需要同时保留组合级指标和单交易对明细，避免只展示汇总收益而丢失数据质量证据。

## 第 6 章：结果交付与基因角色三态

进化产出的基因角色有三态：`challenger`、`champion`、`retired`。角色变化必须通过明确状态机完成，不允许直接覆盖当前冠军。

| 角色 | 含义 | 可写方 |
|---|---|---|
| `challenger` | 进化产出，等待人工审批 | Lab Evolution |
| `champion` | 当前活跃冠军，可供实例绑定或更新 | 人工 Promote 事务 |
| `retired` | 历史冠军，仅供审计和回溯 | 人工 Promote 事务 |

进化任务完成后，只能写入 `challenger`，并附带完整评分报告、窗口明细、出生点、数据覆盖、参数包 JSON、引擎版本和策略版本。它不会自动影响任何生产实例。

人工 Promote 流程必须在 DB 事务内执行：锁定目标策略和标的的当前 champion，把旧 champion 改为 `retired`，把选定 challenger 改为 `champion`，记录操作者、时间和原因。事务提交后立即失效 Redis 冠军缓存。

如果 Promote 失败，旧 champion 必须保持不变。任何情况下都不能出现同一策略同一标的多个活跃 champion，除非未来文档明确引入分组冠军或灰度冠军。

## 第 7 章：HTTP 触发契约

进化 HTTP 接口只在 `app_role=lab` 或 `app_role=dev` 下开放。`app_role=saas` 可以读取必要的报告，但不得创建或执行进化任务。

### 7.1 `POST /api/v1/evolution/tasks`

创建进化任务。请求参数：

| 字段 | 类型 | 默认值 | 约束 | 说明 |
|---|---|---:|---|---|
| `strategy_id` | string | 无 | 必填 | 策略唯一标识 |
| `symbol` | string | 无 | 必填 | 交易对，如 `BTCUSDT` |
| `pop_size` | int | 300 | `[10, 500]` | 种群大小 |
| `max_generations` | int | 25 | `[5, 50]` | 最大代数 |
| `spawn_mode` | enum | `inherit` | `inherit`/`random_once`/`manual` | 出生点模式 |
| `spawn_point` | object/null | null | `manual` 时必填 | Epoch 冻结出生点 |
| `seed` | int64/null | null | 可选 | 复现实验随机种子 |
| `notes` | string/null | null | 可选 | 人工备注，不参与评分 |

接口返回任务 ID、排队状态、解析后的出生点摘要和数据覆盖预检查结果。任务创建不得立即 Promote，也不得下发交易指令。

### 7.2 `GET /api/v1/evolution/tasks`

查询任务列表。返回结构至少包含：

| 字段 | 类型 | 说明 |
|---|---|---|
| `tasks` | array | 任务摘要列表 |
| `tasks[].id` | string | 任务 ID |
| `tasks[].strategy_id` | string | 策略标识 |
| `tasks[].symbol` | string | 交易对 |
| `tasks[].status` | enum | `queued`、`running`、`succeeded`、`failed`、`canceled` |
| `tasks[].pop_size` | int | 种群规模 |
| `tasks[].max_generations` | int | 最大代数 |
| `tasks[].current_generation` | int | 当前代数 |
| `tasks[].best_score` | float/null | 当前或最终最佳总分 |
| `tasks[].best_gene_id` | string/null | 最佳 challenger 或临时基因标识 |
| `tasks[].started_at` | timestamp/null | 开始时间 |
| `tasks[].finished_at` | timestamp/null | 完成时间 |
| `tasks[].error` | string/null | 失败摘要 |

任务详情接口可以在后续扩展，但必须至少能返回窗口评分明细、fatal 信息、缓存命中率、Mutation Ramp 轨迹、数据覆盖和最终 ParamPack 摘要。任何返回给普通用户的文本必须避免裸公式堆叠，内部诊断接口可保留机器字段。
