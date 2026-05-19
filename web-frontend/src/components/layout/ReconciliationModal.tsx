import { AlertTriangle } from "lucide-react";
import { Button } from "../ui/button";
import { Dialog } from "../ui/dialog";

type ReconciliationModalProps = {
  open: boolean;
  reason?: string;
  onClose: () => void;
};

export function ReconciliationModal({ open, reason, onClose }: ReconciliationModalProps) {
  return (
    <Dialog open={open} title="需要人工对账" onClose={onClose}>
      <div className="flex gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-warning/20 bg-warning/10 text-warning">
          <AlertTriangle className="h-5 w-5" />
        </div>
        <div>
          <p className="text-sm leading-6 text-text-main">
            系统检测到账户资产与策略账本需要人工确认。请先核对 Agent 上报余额，再恢复自动运行。
          </p>
          {reason && <p className="mt-3 rounded-md bg-white/[0.03] p-3 text-xs leading-5 text-text-muted">{reason}</p>}
          <div className="mt-5 flex justify-end">
            <Button variant="primary" onClick={onClose}>
              我知道了
            </Button>
          </div>
        </div>
      </div>
    </Dialog>
  );
}
