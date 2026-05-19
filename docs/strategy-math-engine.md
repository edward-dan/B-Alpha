# 策略数学引擎

本文档是策略逻辑的数学规格书，定义 `Step(StrategyInput) -> StrategyOutput` 的输入输出契约、资产状态语义、市场状态感知、宏观 DCA、微观 Sigmoid 动态天平、DeadBTC 释放规则和可进化参数边界。任何策略实现必须先满足本文档，再进入代码。

## 第 0 章：引擎身份

策略数学引擎是纯函数无状态转换器。唯一入口是 `Step(StrategyInput) -> StrategyOutput`，其中“无状态”指函数不读取进程外状态、不依赖隐式内存、不访问 I/O；必要运行状态必须显式放入 `StrategyInput`，下一状态必须显式返回到 `StrategyOutput`。

`Step()` 的职责是根据当前市场序列、资产三态、运行状态、策略参数和交易约束，计算目标仓位、交易意图、账本语义转换和下一份运行状态。`Step()` 不知道自己处于回测还是实盘，不能出现环境分支。

复利前置条件是策略存在的前提。所有订单规模和仓位目标必须基于 `TotalEquity`、`SpendableUSDT`、当前权重或注资计划等可随权益变化的量计算，禁止把核心买卖规模写成与账户权益无关的固定脚本。

`StrategyInput` 至少包含：标的、当前已闭合 K 线序列降级后的价格与时间戳、当前价格、Portfolio State、Runtime State、Chromosome 参数、出生点参数、交易精度、手续费假设和当前聚合桶时间。所有 K 线必须是完全闭合 K 线，不能使用未闭合实时 bar。

`StrategyOutput` 至少包含：宏观交易意图列表、微观交易意图列表、DeadBTC 释放或冷封存等账本转换、下一份 Runtime State、审计原因码、是否跳过交易及跳过原因。输出只能表达意图，不能执行 I/O。

## 第 1 章：资产结构三态（Portfolio State）

Portfolio State 把 BTC 仓位拆成三种语义：`DeadBTC`、`FloatBTC`、`ColdSealedBTC`。三态代表不同的策略权利，而不是不同链上钱包或交易所子账户；真实交易所余额由 Agent 上报，语义拆分由 SaaS 账本维护。

`DeadBTC` 是宏观底仓，只由宏观引擎买入，默认只进不出。它表达长期复利积累和抗短期噪音的核心仓位，不能被微观引擎直接卖出。

`FloatBTC` 是微观浮动仓，可被微观引擎买卖。它承担 Sigmoid 动态天平的弹性调仓职责，通过在目标权重和当前权重之间收敛来捕捉中短期波动。

`ColdSealedBTC` 是冷封存仓，永不释放、永不卖出、永不参与目标权重计算。它只能来自明确的账本封存动作，用于把一部分历史收益永久移出策略风险面。

通用公式如下，所有金额使用同一计价货币 USDT，`Price` 为当前完全闭合 bar 的收盘价或 SaaS 侧选定的保守执行参考价：

```text
TotalBTC = DeadBTC + FloatBTC + ColdSealedBTC

TotalEquity =
  AvailableUSDT + LockedUSDT
  + TotalBTC * Price

ReserveFloor =
  TotalEquity * micro_reserve_pct

SpendableUSDT =
  max(0, AvailableUSDT - ReserveFloor - PendingBuyUSDT)

CurrentMicroWeight =
  if TotalEquity > 0:
    FloatBTC * Price / TotalEquity
  else:
    0
```

`micro_reserve_pct` 表示微观引擎必须保留的 USDT 安全垫比例，默认值为 `0.08`。它是可进化参数，参与染色体交叉变异，硬边界为 `[0.02, 0.30]`；过低会让微观引擎失去回补能力，过高会让浮动仓长期低效。

宏观引擎也必须尊重 `SpendableUSDT`。当 `SpendableUSDT <= 0` 时，任何买入意图都必须归零；当可卖 `FloatBTC <= 0` 时，任何微观卖出意图都必须归零，除非同一 `Step()` 明确输出了合法的 `DeadBTC -> FloatBTC` 释放转换。

## 第 2 章：市场状态感知层

市场状态感知层把完全闭合价格序列转换为少量无量纲状态特征，并输出 `MarketRegime` 与 `MarketBetaMultiplier`。它不直接下单，只影响宏观 DCA 的倍率、微观 Sigmoid 的响应速度以及粉尘订单是否允许穿透。

基础特征全部使用比率、对数收益率或标准化偏离：

```text
r[t] = ln(Close[t] / Close[t-1])

TrendFast = ln(EMA_fast / EMA_mid)
TrendSlow = ln(EMA_mid / EMA_long)
Momentum = ln(Close[t] / Close[t - momentum_n])

VolShort = stdev(r, vol_short_n)
VolLong = max(stdev(r, vol_long_n), epsilon)
VolatilityRatio = clip(VolShort / VolLong, 0.1, 3.0)

DrawdownFromHigh = 1 - Close[t] / rolling_max(Close, drawdown_n)
ReturnZ = r[t] / max(VolLong, epsilon)
```

市场状态分为五类，按优先级从上到下判断。优先级存在的原因是极端风险状态必须覆盖普通牛熊判断，安静态必须覆盖无趋势的小波动环境。

| 状态 | 判断逻辑 | 策略含义 |
|---|---|---|
| `STRESS_CRASH` | `DrawdownFromHigh >= stress_drawdown_pct`，或 `ReturnZ <= -stress_return_z` 且 `VolatilityRatio >= stress_vol_ratio_min` | 极端下跌或波动冲击；宏观放慢但不停止，微观降低追涨杀跌风险 |
| `QUIET` | `abs(TrendFast) <= quiet_trend_threshold`，且 `abs(Momentum) <= quiet_momentum_threshold`，且 `VolatilityRatio <= quiet_vol_ratio_max` | 无趋势低波动；微观粉尘订单一律归零 |
| `BULL_TREND` | `TrendFast >= bull_trend_min`，且 `TrendSlow >= 0`，且 `Momentum >= bull_momentum_min` | 中长期上行；宏观维持低速 DCA，微观可提高响应 |
| `BEAR_TREND` | `TrendFast <= -bear_trend_min`，且 `TrendSlow <= 0`，且 `Momentum <= -bear_momentum_min` | 中长期下行；宏观按估值折扣分批吸入，微观避免过度满仓 |
| `TRANSITION` | 以上均不满足 | 趋势切换或噪音区；使用中性倍率 |

安静态的硬规则是：如果理论订单金额低于交易所最小订单阈值，则归零，不允许楔形区强制最小订单。安静态不是“不交易”，而是禁止微观粉尘为了噪声突破而制造手续费和审计噪音。

`MarketBetaMultiplier` 由状态给出默认倍率：`QUIET=0.60`、`TRANSITION=1.00`、`BULL_TREND=1.15`、`BEAR_TREND=0.85`、`STRESS_CRASH=0.50`。这些倍率本身可进化，但必须保持非负，并通过硬边界限制在 `[0.30, 2.00]`。

## 第 3 章：信号与目标函数框架

策略的目标不是预测下一根 K 线，而是在长期复利约束下，让宏观底仓和微观浮仓协同提高风险调整后的资产增长。宏观引擎负责把外部注资和权益增长转化为长期底仓，微观引擎负责围绕目标权重做弹性再平衡。

微观信号可被理解为一个简化 MPC 概念层：每个 `Step()` 不求解完整未来路径，只用当前无量纲特征估计“现在的 FloatBTC 权重应当偏高还是偏低”。Sigmoid 目标权重提供连续、可微、可压缩的动作空间，避免离散状态机频繁翻转。

适应度目标由进化引擎定义为多窗口相对 Ghost DCA 的超额收益和超额回撤惩罚。策略数学层只负责提供可进化参数空间和确定性 `Step()` 输出，不在 `Step()` 内读取适应度或进化任务状态。

所有信号必须可解释为无量纲数值。允许使用对数价格偏离、对数动量、收益率标准差比、回撤比例、权重偏离和 z-score；禁止把绝对价格、绝对涨跌额或与标的单位绑定的魔法数字作为跨周期核心逻辑。

## 第 4 章：宏观引擎

宏观引擎只负责长期 DCA 买入 `DeadBTC`，不负责卖出。它关注长期趋势、估值偏离和回撤阶段，用权益比例和注资节奏决定每个宏观周期最多投入多少 USDT。

宏观基础预算由出生点参数和当前权益共同决定：

```text
PeriodBudgetFromInjection =
  MonthlyInjectUSDT * macro_interval_days / 30

PeriodBudgetFromEquity =
  TotalEquity * macro_equity_pct_per_period

MacroBaseBudget =
  max(PeriodBudgetFromInjection, PeriodBudgetFromEquity)
```

长期估值折扣和过热惩罚使用无量纲表达：

```text
ValueGap = max(0, ln(EMA_long / Close[t]))
OverheatGap = max(0, ln(Close[t] / EMA_long))
DrawdownScore = clip(DrawdownFromHigh / macro_drawdown_ref, 0, 1)

MacroMultiplier =
  RegimeMacroMultiplier
  * (1 + macro_value_boost * ValueGap + macro_drawdown_boost * DrawdownScore)
  * max(0, 1 - macro_overheat_penalty * OverheatGap)
```

宏观订单金额为：

```text
MacroBuyUSDT =
  min(
    SpendableUSDT * macro_max_spendable_pct,
    MacroBaseBudget * MacroMultiplier
  )
```

当 `MacroBuyUSDT` 小于交易所最小订单金额时，宏观订单归零并记录原因码。宏观引擎不能为了凑单突破 `SpendableUSDT`，不能使用 `ColdSealedBTC`，不能卖出 `DeadBTC`。

默认状态倍率为：`BULL_TREND=0.70`、`TRANSITION=1.00`、`QUIET=0.90`、`BEAR_TREND=1.20`、`STRESS_CRASH=0.50`。含义是牛市不过度追高，熊市按折扣增强吸入，极端暴跌时减速分批，避免一次性耗尽现金。

宏观周期由 `macro_interval_days` 控制，默认 7 天。SaaS 可以每天或每小时扫描实例，但宏观引擎只有在距离上次宏观买入已满该周期，且当前聚合桶是完全闭合桶时，才允许产生新的宏观买入意图。

宏观买入的 `lot_type` 必须是 `DEAD_STACK`，成交后由 SaaS 账本增加 `DeadBTC`。宏观引擎产出的每笔买入都必须带有原因码，如 `MACRO_DCA_BASE`、`MACRO_DRAWDOWN_BOOST`、`MACRO_OVERHEAT_SKIP`。

## 第 5 章：微观引擎

微观引擎采用 Sigmoid 动态天平。它从无量纲市场特征合成一个标量 `Signal`，再把 `Signal`、当前浮仓权重和市场状态倍率映射成目标浮仓权重。

### 5.1 信号公式

微观信号的符号约定是：`Signal > 0` 倾向减仓，`Signal < 0` 倾向加仓。特征定义如下：

```text
X_dev = clip(ln(Close[t] / EMA_mid) / max(VolLong, epsilon), -3, 3)
X_mom = clip(ln(Close[t] / Close[t - momentum_n]) / max(VolLong * sqrt(momentum_n), epsilon), -3, 3)
X_accel = clip(X_mom - X_mom_prev, -3, 3)
X_vol = clip(VolatilityRatio - 1, -2, 2)

SignalRaw =
  w_dev * X_dev
  + w_mom * X_mom
  + w_accel * X_accel
  + w_vol * X_vol

Signal = clip(SignalRaw, -signal_clip, signal_clip)
```

GA 可以决定每个权重的正负和大小，因此同一框架可表达均值回归、动量跟随或混合信号。`signal_clip` 防止极端特征把目标权重瞬间推到边界。

### 5.2 Sigmoid 目标权重公式

```text
CurrentWeight = FloatBTC * Price / TotalEquity

EffectiveBeta = max(0.01, beta * MarketBetaMultiplier)
InventoryBias = clamp(CurrentWeight, 0, 1) - 0.5
Exponent = EffectiveBeta * Signal + gamma * InventoryBias

TargetWeight = 1 / (1 + exp(Exponent))
TargetWeight = clamp(TargetWeight, micro_weight_min, micro_weight_max)
```

公式解释如下：当 `Signal` 为正时，`Exponent` 增大，`TargetWeight` 低于 0.5，微观引擎倾向减仓；当 `Signal` 为负时，`Exponent` 减小，`TargetWeight` 高于 0.5，微观引擎倾向加仓。

`beta` 是激进系数，越大目标权重越容易靠近 0 或 1。`gamma` 是库存偏置系数，`gamma > 0` 时会对过高或过低的浮仓施加均值回归力，防止 FloatBTC 长期漂到极端。

### 5.3 理论订单计算

```text
DeltaWeight = TargetWeight - CurrentWeight
TheoreticalUSD = DeltaWeight * TotalEquity
```

当 `TheoreticalUSD > 0` 时，微观引擎意图买入 `FLOATING`，实际买入金额为 `min(TheoreticalUSD, SpendableUSDT)`。当 `TheoreticalUSD < 0` 时，微观引擎意图卖出 `FLOATING`，实际卖出数量为 `min(FloatBTC, abs(TheoreticalUSD) / Price)`。

任何买入不得突破 `SpendableUSDT`，任何卖出不得突破可卖 `FloatBTC`。微观引擎不能直接卖出 `DeadBTC` 或 `ColdSealedBTC`；若需要更多可卖库存，必须由第 6 章释放规则在同一 `Step()` 中先输出账本转换。

### 5.4 楔形区过滤规则

设 `MinOrderUSDT` 为交易所或 SaaS 传入的最小订单金额，`WedgeWeightThreshold` 和 `WedgeVolatilityThreshold` 为可进化参数。粉尘拦截规则如下：

```text
if abs(TheoreticalUSD) >= MinOrderUSDT:
  allow normal order

else if abs(TheoreticalUSD) > 0
  and MarketRegime != QUIET
  and (
    abs(DeltaWeight) >= WedgeWeightThreshold
    or VolatilityRatio >= WedgeVolatilityThreshold
  ):
  force order to MinOrderUSDT in the same direction, subject to SpendableUSDT/FloatBTC limits

else:
  order = 0
```

安静态下微观粉尘订单一律归零，即使满足楔形突破条件也不强制最小订单。楔形区只用于非安静态的临界突破，目的是避免目标权重已经明显偏离却因交易所最小金额长期无法修正。

## 第 6 章：DeadBTC 释放规则

`DeadBTC` 释放指 SaaS 语义账本中的 `DeadBTC -> FloatBTC` 转换，不是交易所卖出。释放动作本身不产生 `TradeCommand`，只让后续微观引擎在必要时拥有可卖浮动库存。

默认情况下，宏观底仓长期只进不出。只有同时满足“成熟”“盈利或过热”“微观需要库存”“风险不在崩盘中”四类条件时，才允许释放一部分 `DeadBTC`。

释放判断如下：

```text
Mature =
  weighted_average_dead_lot_age_days >= release_min_age_days

Profitable =
  ln(Price / DeadBTCWeightedCost) >= release_profit_log_min

Overheated =
  MarketRegime == BULL_TREND
  and ln(Price / EMA_long) >= release_overheat_log_gap

MicroNeedsSellableInventory =
  TargetWeight < CurrentMicroWeight
  or CurrentMicroWeight >= micro_release_inventory_ceiling

RiskAllowed =
  MarketRegime != STRESS_CRASH
```

当 `Mature && RiskAllowed && MicroNeedsSellableInventory && (Profitable || Overheated)` 成立时，允许释放：

```text
ReleaseBTC =
  min(
    DeadBTC * release_max_pct_per_step,
    max(0, DeadBTC - dead_btc_floor),
    release_target_float_weight_gap * TotalEquity / Price
  )
```

`dead_btc_floor` 是宏观底仓最低保留量，可以由出生点或实例配置给出；`release_max_pct_per_step` 默认不超过 `0.10`，避免一次性把底仓变成浮仓。释放必须写审计原因码，如 `DEAD_RELEASE_OVERHEAT` 或 `DEAD_RELEASE_PROFIT_MATURED`。

`ColdSealedBTC` 永不释放。`STRESS_CRASH` 或 `BEAR_TREND` 下默认禁止释放，避免在恐慌阶段把底仓交给微观卖出；如未来需要紧急偿付或人工干预，必须先扩展本文档，不得在代码中临时开口子。

## 第 7 章：可进化参数契约（Chromosome）

Chromosome 是 GA 代内交叉、变异、指纹和适应度评估的对象。出生点参数是 Epoch 级冻结上下文，全种群共享，不参与代内交叉变异，也不进入基因组指纹。

### 7.1 染色体字段清单

| 名称 | 类型 | 默认值 | 边界 | 语义 |
|---|---|---:|---|---|
| `micro_reserve_pct` | float | 0.08 | `[0.02, 0.30]` | 微观 USDT 安全垫占总权益比例 |
| `ema_fast_n` | int | 24 | `[6, 96]` | 快速 EMA 窗口，按基础 bar 数表示 |
| `ema_mid_n` | int | 168 | `[48, 720]` | 中期 EMA 窗口 |
| `ema_long_n` | int | 720 | `[240, 4320]` | 长期 EMA 窗口 |
| `momentum_n` | int | 72 | `[12, 720]` | 动量观察窗口 |
| `vol_short_n` | int | 24 | `[6, 168]` | 短波动窗口 |
| `vol_long_n` | int | 720 | `[240, 4320]` | 长波动窗口 |
| `drawdown_n` | int | 720 | `[240, 4320]` | 回撤观察窗口 |
| `quiet_trend_threshold` | float | 0.010 | `[0.003, 0.050]` | 安静态趋势阈值 |
| `quiet_momentum_threshold` | float | 0.015 | `[0.005, 0.080]` | 安静态动量阈值 |
| `quiet_vol_ratio_max` | float | 0.75 | `[0.30, 1.20]` | 安静态最大波动比 |
| `bull_trend_min` | float | 0.012 | `[0.003, 0.080]` | 牛市趋势最小强度 |
| `bull_momentum_min` | float | 0.020 | `[0.005, 0.120]` | 牛市动量最小强度 |
| `bear_trend_min` | float | 0.012 | `[0.003, 0.080]` | 熊市趋势最小强度 |
| `bear_momentum_min` | float | 0.020 | `[0.005, 0.120]` | 熊市动量最小强度 |
| `stress_drawdown_pct` | float | 0.22 | `[0.08, 0.45]` | 极端回撤阈值 |
| `stress_return_z` | float | 3.0 | `[1.5, 6.0]` | 单 bar 负收益 z-score 阈值 |
| `stress_vol_ratio_min` | float | 1.60 | `[1.00, 3.00]` | 极端状态最小波动比 |
| `market_beta_quiet` | float | 0.60 | `[0.30, 1.20]` | 安静态微观 beta 倍率 |
| `market_beta_bull` | float | 1.15 | `[0.50, 2.00]` | 牛市微观 beta 倍率 |
| `market_beta_bear` | float | 0.85 | `[0.30, 1.50]` | 熊市微观 beta 倍率 |
| `market_beta_stress` | float | 0.50 | `[0.20, 1.00]` | 极端状态微观 beta 倍率 |
| `w_dev` | float | 0.80 | `[-3.00, 3.00]` | 价格偏离信号权重 |
| `w_mom` | float | -0.35 | `[-3.00, 3.00]` | 动量信号权重 |
| `w_accel` | float | 0.20 | `[-2.00, 2.00]` | 动量变化信号权重 |
| `w_vol` | float | 0.15 | `[-2.00, 2.00]` | 波动变化信号权重 |
| `signal_clip` | float | 4.0 | `[1.0, 8.0]` | 合成信号截断上限 |
| `beta` | float | 1.60 | `[0.10, 8.00]` | Sigmoid 激进系数 |
| `gamma` | float | 1.00 | `[0.00, 5.00]` | 库存偏置系数 |
| `micro_weight_min` | float | 0.02 | `[0.00, 0.30]` | 微观目标权重下界 |
| `micro_weight_max` | float | 0.85 | `[0.40, 1.00]` | 微观目标权重上界 |
| `wedge_weight_threshold` | float | 0.015 | `[0.002, 0.080]` | 楔形区最小权重偏离 |
| `wedge_volatility_threshold` | float | 1.35 | `[1.00, 3.00]` | 楔形区波动突破阈值 |
| `macro_interval_days` | int | 7 | `[1, 30]` | 宏观 DCA 周期 |
| `macro_equity_pct_per_period` | float | 0.010 | `[0.001, 0.050]` | 每宏观周期按权益计算的基础预算 |
| `macro_max_spendable_pct` | float | 0.35 | `[0.05, 0.80]` | 单次宏观最多使用可花 USDT 比例 |
| `macro_drawdown_ref` | float | 0.35 | `[0.10, 0.70]` | 回撤增强归一化参考 |
| `macro_value_boost` | float | 2.00 | `[0.00, 8.00]` | 长期低估时的 DCA 增强 |
| `macro_drawdown_boost` | float | 1.50 | `[0.00, 6.00]` | 回撤时的 DCA 增强 |
| `macro_overheat_penalty` | float | 2.00 | `[0.00, 8.00]` | 高于长期均线时的 DCA 抑制 |
| `macro_mult_bull` | float | 0.70 | `[0.20, 1.50]` | 牛市宏观倍率 |
| `macro_mult_bear` | float | 1.20 | `[0.50, 2.50]` | 熊市宏观倍率 |
| `macro_mult_quiet` | float | 0.90 | `[0.30, 1.50]` | 安静态宏观倍率 |
| `macro_mult_stress` | float | 0.50 | `[0.10, 1.20]` | 极端状态宏观倍率 |
| `release_min_age_days` | int | 365 | `[30, 1460]` | DeadBTC 可释放的最小加权年龄 |
| `release_profit_log_min` | float | 0.35 | `[0.05, 1.50]` | 可释放的最小对数盈利 |
| `release_overheat_log_gap` | float | 0.30 | `[0.05, 1.20]` | 过热释放阈值 |
| `release_max_pct_per_step` | float | 0.05 | `[0.00, 0.20]` | 单次最多释放 DeadBTC 比例 |
| `release_target_float_weight_gap` | float | 0.10 | `[0.01, 0.40]` | 释放后补足的浮仓权重缺口上限 |

### 7.2 硬边界约束

所有百分比、权重、倍率和窗口必须先 Clamp 到字段边界，再进入 `Step()`。Clamp 之后仍不合法的基因必须由 Validate 判为非法，适应度直接返回 fatal 或拒绝入库。

`TotalEquity <= 0` 时不得产生交易意图。`SpendableUSDT <= 0` 时不得产生买入意图。`FloatBTC <= 0` 且无合法释放时不得产生卖出意图。`TargetWeight` 必须在 `[0, 1]` 内，且再受 `micro_weight_min <= micro_weight_max` 约束。

任何订单都不得突破交易所最小精度、最小金额、可用余额、可卖浮仓和命令过期时间。策略只输出理论意图，SaaS 翻译命令时还必须再次做执行层约束。

`ColdSealedBTC` 不得被释放、卖出或纳入微观权重。`DeadBTC` 不得被微观直接卖出，只能先通过第 6 章规则释放为 `FloatBTC`。

### 7.3 结构约束

EMA 窗口必须满足 `ema_fast_n < ema_mid_n < ema_long_n`，并建议修复为 `ema_mid_n >= 2 * ema_fast_n`、`ema_long_n >= 2 * ema_mid_n`。若随机采样或交叉后破坏顺序，Clamp 必须先排序再扩距。

波动窗口必须满足 `vol_long_n >= 3 * vol_short_n`。动量窗口必须满足 `ema_fast_n <= momentum_n <= ema_long_n`，避免动量特征比快速均线更短或比长期趋势还慢。

微观权重上下界必须满足 `0 <= micro_weight_min < micro_weight_max <= 1`，且 `micro_weight_max` 不得高到让 `ReserveFloor` 永远无法维持。若两者交叉，Clamp 必须固定下界后提高上界或回退默认值。

时间周期相对论锁：所有窗口字段以基础 bar 数保存，但语义是墙上时间长度。实例基础周期变化时，应按墙上时间换算为新的 bar 数并重新 Clamp，而不是把“24 根 bar”从小时级直接解释成日级。

### 7.4 出生点参数

出生点参数在 Epoch 启动时冻结，全种群共享，不参与代内交叉变异，不进入 Fingerprint。它们描述评估起点和资金制度，而不是策略 DNA。

出生点参数包括：交易对、计价资产、基础周期、初始 `AvailableUSDT`、初始 `DeadBTC`、初始 `FloatBTC`、初始 `ColdSealedBTC`、`MonthlyInjectUSDT`、手续费假设、滑点假设、交易所最小订单金额、lot step、lot min、起始时间、评估窗口集合和风险硬阈值。

`MonthlyInjectUSDT`、初始资金和交易精度会显著影响回测结果，但它们代表“出生环境”，不是代内可进化基因。若要探索不同出生点，应启动新的 Epoch，而不是把它们放入 Chromosome。
