import { cva, type VariantProps } from "class-variance-authority";
import * as React from "react";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
  {
    variants: {
      variant: {
        neutral: "bg-bg-tertiary text-text-secondary border border-border-primary",
        success: "bg-success-500/10 text-success-500 border border-success-500/20",
        warning: "bg-warning-500/10 text-warning-500 border border-warning-500/20",
        danger: "bg-error-500/10 text-error-500 border border-error-500/20",
        info: "bg-info-500/10 text-info-500 border border-info-500/20",
        brand: "bg-brand-500/10 text-brand-300 border border-brand-500/20",
      },
    },
    defaultVariants: { variant: "neutral" },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}
