export interface MapPin {
  x: number; // 0-100
  y: number; // 0-100
  color?: string;
  label?: string;
}

// Decorative "city" geometry so the panel reads as a map rather than a grid.
const blocks = [
  [8, 8, 16, 10], [28, 6, 12, 14], [46, 10, 14, 9], [66, 6, 12, 11], [84, 12, 10, 12],
  [10, 34, 14, 12], [30, 40, 12, 10], [52, 34, 16, 11], [72, 42, 14, 10], [88, 36, 8, 12],
  [16, 74, 12, 9], [36, 70, 14, 11], [58, 76, 12, 8], [76, 70, 14, 10],
] as const;
const roads = [
  "M0 30 H100", "M0 62 H100", "M24 0 V100", "M62 0 V100", "M0 8 L100 52", "M0 96 L100 40",
];

/** Stylised map panel: streets, city blocks, a water body, a dashed delivery
 * route and teardrop pins. Aspect ratio is preserved so pins stay round. */
export function MapPins({ pins, route, height = 260 }: { pins: MapPin[]; route?: MapPin[]; height?: number }) {
  const sy = (y: number) => y * 0.6; // map 0-100 into the 0-60 viewBox

  return (
    <svg viewBox="0 0 100 60" className="w-full rounded-[var(--radius-vela-lg)] bg-bg-inset" style={{ height }} preserveAspectRatio="xMidYMid slice">
      {/* water */}
      <path d="M0 60 L0 40 Q18 34 30 44 T60 46 Q80 47 100 38 L100 60 Z" fill="var(--info)" opacity="0.10" />
      {/* parks / greens */}
      <rect x="4" y="18" width="12" height="9" rx="2" fill="var(--ok)" opacity="0.12" />
      <rect x="80" y="4" width="14" height="8" rx="2" fill="var(--ok)" opacity="0.12" />
      {/* city blocks */}
      {blocks.map((b, i) => (
        <rect key={i} x={b[0]} y={sy(b[1])} width={b[2]} height={sy(b[3])} rx="1.2" fill="var(--line-2)" opacity="0.5" />
      ))}
      {/* streets */}
      {roads.map((d, i) => (
        <path key={i} d={d} fill="none" stroke="var(--line-2)" strokeWidth="0.7" opacity="0.7" />
      ))}

      {/* delivery route */}
      {route && route.length > 1 && (
        <>
          <path d={`M ${route.map((p) => `${p.x} ${sy(p.y)}`).join(" L ")}`} fill="none" stroke="var(--acc)" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" opacity="0.18" />
          <path d={`M ${route.map((p) => `${p.x} ${sy(p.y)}`).join(" L ")}`} fill="none" stroke="var(--acc)" strokeWidth="0.9" strokeLinecap="round" strokeLinejoin="round" strokeDasharray="2.4 1.8" />
        </>
      )}

      {/* pins (teardrop) */}
      {pins.map((pin, i) => {
        const c = pin.color ?? "var(--acc)";
        return (
          <g key={i} transform={`translate(${pin.x}, ${sy(pin.y)})`}>
            <ellipse cx="0" cy="0.6" rx="1.7" ry="0.6" fill="rgba(0,0,0,.35)" />
            <path d="M0 0 C -2 -2.6 -2.4 -4.4 0 -5.6 C 2.4 -4.4 2 -2.6 0 0 Z" fill={c} stroke="#fff" strokeWidth="0.35" />
            <circle cx="0" cy="-3.6" r="0.95" fill="#fff" />
          </g>
        );
      })}
    </svg>
  );
}
