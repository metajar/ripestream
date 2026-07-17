import { cn } from "@/lib/utils";

/** Networking-themed spinner: an RJ45 plug on a Cat5e cable that rotates. */
export function CableSpinner({
  className,
  size = 20,
}: {
  className?: string;
  size?: number;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={cn("animate-spin text-brand-500", className)}
      aria-hidden
    >
      {/* Cable loop (rotates with the whole mark) */}
      <path
        d="M16 28c-5.5 0-10-3.6-10-8.5 0-3.2 1.8-6 4.6-7.5"
        stroke="currentColor"
        strokeWidth="2.2"
        strokeLinecap="round"
        opacity="0.35"
      />
      <path
        d="M16 28c5.5 0 10-3.6 10-8.5 0-3.2-1.8-6-4.6-7.5"
        stroke="currentColor"
        strokeWidth="2.2"
        strokeLinecap="round"
        opacity="0.55"
      />
      {/* Twisted-pair hint near the plug */}
      <path
        d="M14.2 14.2c.6-.9 1.6-1.4 2.6-1.4"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        opacity="0.7"
      />
      {/* RJ45 connector body */}
      <rect
        x="11"
        y="4"
        width="10"
        height="9.5"
        rx="1.2"
        fill="currentColor"
        opacity="0.92"
      />
      {/* Clip latch */}
      <path
        d="M13.2 4V2.8c0-.4.3-.8.8-.8h4c.4 0 .8.4.8.8V4"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
        opacity="0.75"
      />
      {/* Gold contacts */}
      <g fill="#fdb022">
        <rect x="12.4" y="10.2" width="1.1" height="2.4" rx="0.3" />
        <rect x="14" y="10.2" width="1.1" height="2.4" rx="0.3" />
        <rect x="15.6" y="10.2" width="1.1" height="2.4" rx="0.3" />
        <rect x="17.2" y="10.2" width="1.1" height="2.4" rx="0.3" />
        <rect x="18.8" y="10.2" width="1.1" height="2.4" rx="0.3" />
      </g>
      {/* Strain-relief boot */}
      <path
        d="M12.5 13.5h7c.6 0 1 .4 1 1v1.2c0 .8-.7 1.4-1.5 1.4h-6c-.8 0-1.5-.6-1.5-1.4V14.5c0-.6.4-1 1-1Z"
        fill="currentColor"
        opacity="0.7"
      />
    </svg>
  );
}
