export type AppRole = "saas" | "lab" | "dev";

export type AppFeature = "dashboard" | "strategies" | "agents" | "risk" | "backtesting" | "settings";
export type FeatureKey = AppFeature;

export type User = {
  id: number;
  email: string;
  role: string;
  subscription_plan: string;
  subscription_status: string;
  subscription_expires_at?: string | null;
};

export type AuthResponse = {
  token: string;
  user: User;
};

export type StrategyTemplate = {
  id: number;
  created_at?: string;
  updated_at?: string;
  name: string;
  version: string;
  is_spot: boolean;
  manifest: Record<string, unknown>;
};

export type PortfolioState = {
  id?: number;
  usdt_balance?: string | number;
  dead_btc?: string | number;
  float_btc?: string | number;
  cold_sealed_btc?: string | number;
  total_equity?: string | number;
  last_processed_bar_time?: string | null;
};

export type RuntimeState = {
  id?: number;
  state?: Record<string, unknown>;
};

export type StrategyInstanceStatus = "RUNNING" | "STOPPED" | "ERROR" | "DELETED";

export type StrategyInstance = {
  id: number;
  created_at?: string;
  updated_at?: string;
  user_id: number;
  template_id: number;
  template?: StrategyTemplate | null;
  name: string;
  symbol: string;
  interval: string;
  status: StrategyInstanceStatus;
  config?: Record<string, unknown>;
  last_error?: string;
  portfolio?: PortfolioState | null;
  runtime_state?: RuntimeState | null;
};

export type EquitySnapshot = {
  time: string;
  total_equity: number | string;
};

export type DashboardOverview = {
  user_id: number;
  instances_total: number;
  running_count: number;
  stopped_count: number;
  error_count: number;
  total_equity: number;
  usdt_balance: number;
  dead_btc: number;
  float_btc: number;
  cold_sealed_btc: number;
  pending_commands: number;
};

export type TradeRecord = {
  id: number;
  created_at?: string;
  client_order_id: string;
  action: string;
  engine: string;
  symbol: string;
  filled_qty: string | number;
  filled_price: string | number;
  fee: string | number;
  executed_at?: string | null;
};

export type DashboardResponse = {
  overview: DashboardOverview;
  instances: StrategyInstance[];
  recent_trades: TradeRecord[];
};

export type AgentStatus = {
  user_id: number;
  agent_id: string;
  connected: boolean;
};

export type SystemStatus = {
  engine_state: "running" | "paused" | "halted";
  app_role: AppRole;
  features: Partial<Record<AppFeature, boolean>>;
  agent_connected: boolean;
  api_connected?: boolean;
  api_configured?: boolean;
  agent_version?: string;
  last_heartbeat_at?: string | null;
  needs_reconciliation: boolean;
  reconciliation_reason?: string;
  checked_at: string;
};

export type GeneRecord = {
  id: number;
  created_at?: string;
  updated_at?: string;
  strategy_id: string;
  symbol: string;
  role: "challenger" | "champion" | "retired";
  param_pack: Record<string, unknown>;
  score_total: string | number;
  max_drawdown: string | number;
  score_report: Record<string, unknown>;
  promoted_at?: string | null;
  retired_at?: string | null;
};

export type EvolutionTask = {
  id: number;
  created_at?: string;
  updated_at?: string;
  strategy_id: string;
  symbol: string;
  status: "queued" | "running" | "succeeded" | "failed" | "canceled";
  progress: number;
  config: Record<string, unknown>;
  current_generation: number;
  best_score: string | number;
  best_gene_id?: string;
  started_at?: string | null;
  finished_at?: string | null;
  error?: string;
};

export type EvolutionStatus = {
  current_task?: EvolutionTask | null;
  tasks?: EvolutionTask[];
  active_tasks?: EvolutionTask[];
  recent_tasks?: EvolutionTask[];
  challengers?: GeneRecord[];
  [key: string]: unknown;
};

export type BacktestRun = {
  id: number;
  created_at?: string;
  updated_at?: string;
  strategy_id: string;
  symbol: string;
  interval: string;
  status: "running" | "succeeded" | "failed";
  request: Record<string, unknown>;
  result: Record<string, unknown>;
  error?: string;
  started_at?: string | null;
  finished_at?: string | null;
};

export type CreateInstancePayload = {
  template_id: number;
  name: string;
  symbol: string;
  interval: string;
  config: Record<string, unknown>;
  initial_portfolio: {
    usdt_balance: number;
    dead_btc: number;
    float_btc: number;
    cold_sealed_btc: number;
    total_equity: number;
  };
  initial_cost_price: number;
};

export type CreateEvolutionTaskPayload = {
  strategy_id: string;
  symbol: string;
  [key: string]: unknown;
};

export type CreateBacktestPayload = {
  strategy_id: string;
  symbol: string;
  interval: string;
  gene_id?: number;
  param_pack?: Record<string, unknown>;
  limit?: number;
};
