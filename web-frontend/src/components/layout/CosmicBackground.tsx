import type { ReactNode } from "react";
import { motion, useReducedMotion } from "motion/react";

type CosmicBackgroundProps = {
  children: ReactNode;
};

export function CosmicBackground({ children }: CosmicBackgroundProps) {
  const reducedMotion = useReducedMotion();
  const floatTransition = reducedMotion
    ? undefined
    : {
        duration: 18,
        repeat: Infinity,
        repeatType: "reverse" as const,
        ease: "easeInOut" as const
      };

  return (
    <div className="relative min-h-screen overflow-hidden bg-background text-text-main">
      <div className="pointer-events-none fixed inset-0">
        <div className="absolute left-[-12%] top-[-12%] h-96 w-96 rounded-full bg-warm/25 blur-[130px] mix-blend-screen" />
        <div className="absolute bottom-[-16%] right-[-10%] h-[30rem] w-[30rem] rounded-full bg-info/20 blur-[140px] mix-blend-screen" />
        <div className="absolute left-[32%] top-[18%] h-[28rem] w-[28rem] rounded-full bg-accent/15 blur-[120px] mix-blend-screen" />
        <div className="qs-noise absolute inset-0 opacity-[0.15] mix-blend-color-dodge" />
        <motion.div
          className="qs-pentagon absolute left-8 top-[30%] h-28 w-28 border border-accent/30 bg-linear-to-br from-accent/10 to-transparent opacity-50"
          animate={reducedMotion ? undefined : { y: [-10, 18], rotate: [-4, 7] }}
          transition={floatTransition}
        />
        <motion.div
          className="qs-parallelogram absolute right-10 top-[18%] h-24 w-36 border border-warm/30 bg-linear-to-br from-warm/10 to-transparent opacity-50"
          animate={reducedMotion ? undefined : { y: [18, -12], rotate: [5, -5] }}
          transition={floatTransition}
        />
      </div>
      <div className="relative z-10 min-h-screen">{children}</div>
    </div>
  );
}
