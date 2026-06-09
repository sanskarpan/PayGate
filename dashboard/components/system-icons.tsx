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

export function ControlMonolith(props: IconProps) {
  return (
    <svg aria-hidden="true" fill="none" viewBox="0 0 720 720" {...props}>
      <defs>
        <linearGradient id="monoShell" x1="0%" x2="100%" y1="0%" y2="100%">
          <stop offset="0%" stopColor="#ffffff" />
          <stop offset="52%" stopColor="#dde4eb" />
          <stop offset="100%" stopColor="#b4bec8" />
        </linearGradient>
        <linearGradient id="monoPanel" x1="0%" x2="100%" y1="0%" y2="100%">
          <stop offset="0%" stopColor="#20262f" />
          <stop offset="100%" stopColor="#0c1016" />
        </linearGradient>
        <linearGradient id="monoBlue" x1="0%" x2="100%" y1="0%" y2="0%">
          <stop offset="0%" stopColor="#c6f4ff" />
          <stop offset="100%" stopColor="#6d89ff" />
        </linearGradient>
      </defs>
      <ellipse cx="374" cy="650" rx="222" ry="42" fill="#131920" opacity="0.12" />
      <g transform="translate(148 84)">
        <path d="M138 0h232l88 88v338l-94 96H104L0 406V120z" fill="url(#monoShell)" />
        <path d="M150 32h200l68 68v272l-72 74H132L42 356V138z" fill="url(#monoPanel)" />
        <rect x="126" y="96" width="208" height="148" rx="24" fill="#05080c" stroke="#3b4755" strokeWidth="5" />
        <path d="M126 276h230" stroke="#111820" strokeLinecap="round" strokeWidth="10" />
        <path d="M150 318h160" stroke="#eef4f9" strokeLinecap="round" strokeOpacity="0.34" strokeWidth="8" />
        <path d="M152 44h164" stroke="#ffffff" strokeLinecap="round" strokeOpacity="0.5" strokeWidth="7" />
        <path d="M372 152h56" stroke="#6a7683" strokeLinecap="round" strokeWidth="10" />
        <path d="M372 196h34" stroke="#6a7683" strokeLinecap="round" strokeWidth="10" />
        <g opacity="0.9">
          <path d="M164 122h132" stroke="url(#monoBlue)" strokeLinecap="round" strokeWidth="6" />
          <path d="M164 152h102" stroke="#f8fcff" strokeLinecap="round" strokeOpacity="0.8" strokeWidth="5" />
          <path d="M164 182h84" stroke="#9ab0bf" strokeLinecap="round" strokeWidth="5" />
        </g>
        <circle cx="368" cy="94" r="18" fill="#eff4f8" opacity="0.7" />
      </g>
      <g transform="translate(112 472)">
        <path d="M0 0h496l-86 112H86z" fill="#0e1319" opacity="0.88" />
        <path d="M54 28h182" stroke="#d9e2ea" strokeLinecap="round" strokeOpacity="0.28" strokeWidth="8" />
        <path d="M274 28h84" stroke="#7b8895" strokeLinecap="round" strokeWidth="8" />
        <path d="M54 58h296" stroke="#d9e2ea" strokeLinecap="round" strokeOpacity="0.16" strokeWidth="8" />
      </g>
    </svg>
  );
}

export function OrbitalRig(props: IconProps) {
  return (
    <svg aria-hidden="true" fill="none" viewBox="0 0 720 720" {...props}>
      <defs>
        <linearGradient id="rigBody" x1="0%" x2="100%" y1="0%" y2="100%">
          <stop offset="0%" stopColor="#ffffff" />
          <stop offset="54%" stopColor="#d9e2ea" />
          <stop offset="100%" stopColor="#a8b4c0" />
        </linearGradient>
        <linearGradient id="rigShadow" x1="0%" x2="0%" y1="0%" y2="100%">
          <stop offset="0%" stopColor="#0c1219" stopOpacity="0.94" />
          <stop offset="100%" stopColor="#2d333c" stopOpacity="0.86" />
        </linearGradient>
        <linearGradient id="rigAccent" x1="0%" x2="100%" y1="0%" y2="0%">
          <stop offset="0%" stopColor="#5b6bff" />
          <stop offset="100%" stopColor="#97f0ff" />
        </linearGradient>
        <filter id="rigBlur" x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="18" />
        </filter>
      </defs>
      <ellipse cx="364" cy="624" rx="190" ry="46" fill="#0f141b" opacity="0.12" />
      <ellipse cx="364" cy="612" rx="150" ry="22" fill="#6e7b88" opacity="0.18" filter="url(#rigBlur)" />
      <path d="M198 204C276 116 426 96 512 164" opacity="0.18" stroke="url(#rigAccent)" strokeWidth="16" />
      <path d="M186 254C268 154 446 132 546 218" opacity="0.1" stroke="#0f141b" strokeWidth="4" />
      <g transform="translate(204 124)">
        <path
          d="M148 0h108l62 60v178l-74 86H110L0 210V74z"
          fill="url(#rigBody)"
          stroke="#c4d0da"
          strokeWidth="4"
        />
        <path
          d="M138 20h98l44 44v136l-54 62H120L36 192V80z"
          fill="url(#rigShadow)"
          opacity="0.95"
        />
        <rect x="122" y="62" width="132" height="118" rx="28" fill="#090d12" />
        <circle cx="188" cy="121" r="44" fill="#141b22" stroke="#36414d" strokeWidth="5" />
        <circle cx="188" cy="121" r="28" fill="url(#rigAccent)" opacity="0.25" />
        <circle cx="188" cy="121" r="19" fill="#ebf4fb" opacity="0.92" />
        <circle cx="188" cy="121" r="10" fill="#090d12" />
        <path d="M148 24h96" stroke="#eff4f8" strokeLinecap="round" strokeOpacity="0.55" strokeWidth="8" />
        <path d="M86 96h26M86 148h18M264 96h22M270 150h16" stroke="#202833" strokeLinecap="round" strokeWidth="8" />
        <path d="M104 214h168" stroke="#10161d" strokeLinecap="round" strokeWidth="8" />
        <path d="M126 240h118" stroke="#ccd7df" strokeLinecap="round" strokeOpacity="0.45" strokeWidth="6" />
      </g>
      <g transform="translate(240 410)">
        <path d="M54 0 0 110" stroke="#8c9aa6" strokeLinecap="round" strokeWidth="22" />
        <path d="M176 0 228 110" stroke="#8c9aa6" strokeLinecap="round" strokeWidth="22" />
        <path d="M0 110 42 160" stroke="#1c232c" strokeLinecap="round" strokeWidth="18" />
        <path d="M228 110 188 160" stroke="#1c232c" strokeLinecap="round" strokeWidth="18" />
        <path d="M34 160 64 252" stroke="#7f8d98" strokeLinecap="round" strokeWidth="16" />
        <path d="M194 160 164 252" stroke="#7f8d98" strokeLinecap="round" strokeWidth="16" />
        <path d="M63 252 86 304" stroke="#10161d" strokeLinecap="round" strokeWidth="12" />
        <path d="M165 252 142 304" stroke="#10161d" strokeLinecap="round" strokeWidth="12" />
        <circle cx="54" cy="0" r="18" fill="#eff4f8" stroke="#bac6d0" strokeWidth="4" />
        <circle cx="176" cy="0" r="18" fill="#eff4f8" stroke="#bac6d0" strokeWidth="4" />
      </g>
    </svg>
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
