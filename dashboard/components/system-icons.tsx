import type { SVGProps } from "react";

type IconProps = SVGProps<SVGSVGElement> & {
  size?: number;
};

function BaseIcon({
  size = 18,
  viewBox = "0 0 24 24",
  children,
  ...props
}: IconProps & { children: React.ReactNode; viewBox?: string }) {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height={size}
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.7"
      viewBox={viewBox}
      width={size}
      {...props}
    >
      {children}
    </svg>
  );
}

export function BrandEmblem(props: IconProps) {
  return (
    <BaseIcon size={props.size ?? 28} viewBox="0 0 40 40" {...props}>
      <rect x="2.75" y="2.75" width="34.5" height="34.5" rx="7" />
      <path d="M12 10.5v18" />
      <path d="M12 10.5h10.5" />
      <path d="M12 20h14" />
      <path d="M22.5 29.5H12" />
      <path d="M26.5 10.5l4 4-4 4" />
    </BaseIcon>
  );
}

export function RouteIcon({ name, ...props }: IconProps & { name: string }) {
  switch (name) {
    case "overview":
      return (
        <BaseIcon {...props}>
          <path d="M4 12h16" />
          <path d="M12 4v16" />
          <circle cx="12" cy="12" r="7.5" />
        </BaseIcon>
      );
    case "control-plane":
      return (
        <BaseIcon {...props}>
          <rect x="4" y="5" width="16" height="14" rx="3" />
          <path d="M8 9h8" />
          <path d="M8 13h5" />
          <path d="M16.5 12l1.5 1.5-1.5 1.5" />
        </BaseIcon>
      );
    case "observability":
      return (
        <BaseIcon {...props}>
          <path d="M4 15.5h4l2-6 4 9 2.5-5h3.5" />
          <path d="M4 19.5h16" />
        </BaseIcon>
      );
    case "recon":
      return (
        <BaseIcon {...props}>
          <path d="M5 7h7" />
          <path d="M5 12h10" />
          <path d="M5 17h6" />
          <path d="M16 7l3 3-3 3" />
          <path d="M19 14l-3 3 3 3" />
        </BaseIcon>
      );
    case "orders":
      return (
        <BaseIcon {...props}>
          <rect x="5" y="5" width="14" height="14" rx="2.5" />
          <path d="M9 9h6" />
          <path d="M9 13h6" />
        </BaseIcon>
      );
    case "settlements":
      return (
        <BaseIcon {...props}>
          <path d="M5 8h14" />
          <path d="M7 5v6" />
          <path d="M17 5v6" />
          <rect x="5" y="11" width="14" height="8" rx="2.5" />
        </BaseIcon>
      );
    case "payouts":
      return (
        <BaseIcon {...props}>
          <path d="M4 16h10" />
          <path d="M11 9l7 7-7 7" transform="scale(.6) translate(7 1)" />
          <path d="M14 8h5v5" />
        </BaseIcon>
      );
    case "reports":
      return (
        <BaseIcon {...props}>
          <path d="M7 18h10" />
          <path d="M8 14h2" />
          <path d="M12 11h2" />
          <path d="M16 8h1" />
          <path d="M6 19V6h12" />
        </BaseIcon>
      );
    case "mandates":
      return (
        <BaseIcon {...props}>
          <rect x="5" y="6" width="14" height="11" rx="3" />
          <path d="M9 18v2" />
          <path d="M15 18v2" />
          <path d="M8.5 11.5l2 2 4-4" />
        </BaseIcon>
      );
    case "compliance":
      return (
        <BaseIcon {...props}>
          <path d="M12 4l7 3v5c0 4.5-2.8 7.7-7 8.9-4.2-1.2-7-4.4-7-8.9V7l7-3z" />
          <path d="M9.5 12l2 2 3.5-4" />
        </BaseIcon>
      );
    case "risk":
      return (
        <BaseIcon {...props}>
          <path d="M12 4l8 14H4L12 4z" />
          <path d="M12 9v4" />
          <circle cx="12" cy="16" r=".9" fill="currentColor" stroke="none" />
        </BaseIcon>
      );
    case "disputes":
      return (
        <BaseIcon {...props}>
          <path d="M7 7h10v8H9l-4 3V7h2z" />
          <path d="M10 10h4" />
          <path d="M10 13h2" />
        </BaseIcon>
      );
    case "webhooks":
      return (
        <BaseIcon {...props}>
          <path d="M8 8a4 4 0 1 1 4 4h-1" />
          <path d="M16 16a4 4 0 1 1-4-4h1" />
          <path d="M9 15l6-6" />
        </BaseIcon>
      );
    case "gateway":
      return (
        <BaseIcon {...props}>
          <path d="M5 7h14" />
          <path d="M5 12h14" />
          <path d="M5 17h8" />
          <path d="M16 15l3 2-3 2" />
        </BaseIcon>
      );
    case "api-keys":
      return (
        <BaseIcon {...props}>
          <circle cx="9" cy="12" r="3.25" />
          <path d="M12.25 12H20" />
          <path d="M17 12v3" />
          <path d="M14.5 12v2" />
        </BaseIcon>
      );
    case "audit":
      return (
        <BaseIcon {...props}>
          <path d="M7 5h10v14H7z" />
          <path d="M9.5 9h5" />
          <path d="M9.5 13h5" />
        </BaseIcon>
      );
    case "team":
      return (
        <BaseIcon {...props}>
          <circle cx="9" cy="10" r="2.5" />
          <circle cx="16.5" cy="11.5" r="2" />
          <path d="M5.5 18c.8-2.2 2.8-3.5 5.5-3.5s4.7 1.3 5.5 3.5" />
        </BaseIcon>
      );
    default:
      return (
        <BaseIcon {...props}>
          <path d="M5 12h14" />
          <path d="M12 5v14" />
        </BaseIcon>
      );
  }
}

export function WireTunnel(props: IconProps) {
  return (
    <BaseIcon viewBox="0 0 520 520" {...props}>
      <defs>
        <radialGradient id="tunnelFade" cx="50%" cy="50%" r="56%">
          <stop offset="0%" stopColor="currentColor" stopOpacity="0.04" />
          <stop offset="66%" stopColor="currentColor" stopOpacity="0.2" />
          <stop offset="100%" stopColor="currentColor" stopOpacity="0.42" />
        </radialGradient>
      </defs>
      <rect x="0" y="0" width="520" height="520" fill="url(#tunnelFade)" stroke="none" />
      {Array.from({ length: 17 }).map((_, i) => {
        const inset = 18 + i * 14;
        const radius = Math.max(18, 170 - i * 8);
        return (
          <rect
            key={`ring-${i}`}
            x={inset}
            y={inset}
            width={520 - inset * 2}
            height={520 - inset * 2}
            rx={radius}
          />
        );
      })}
      {Array.from({ length: 16 }).map((_, i) => {
        const offset = 20 + i * 30;
        return (
          <g key={`strand-${i}`} opacity={0.82 - i * 0.03}>
            <path d={`M${offset} 0 Q260 260 ${offset} 520`} />
            <path d={`M${520 - offset} 0 Q260 260 ${520 - offset} 520`} />
            <path d={`M0 ${offset} Q260 260 520 ${offset}`} />
            <path d={`M0 ${520 - offset} Q260 260 520 ${520 - offset}`} />
          </g>
        );
      })}
      <rect x="128" y="128" width="264" height="264" rx="96" opacity="0.5" />
    </BaseIcon>
  );
}

export function SignalStamp(props: IconProps) {
  return (
    <BaseIcon size={props.size ?? 88} viewBox="0 0 88 88" {...props}>
      <rect x="5" y="5" width="78" height="78" rx="14" />
      <path d="M17 24h26" />
      <path d="M17 36h40" />
      <path d="M17 48h21" />
      <path d="M52 54l8-8 10 10" />
      <circle cx="61.5" cy="37.5" r="8.5" />
    </BaseIcon>
  );
}
