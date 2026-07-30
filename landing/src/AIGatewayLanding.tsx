import { useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { Link } from "react-router";

// ─── Types ────────────────────────────────────────────────────────────────────

interface Feature {
  icon: ReactNode;
  title: string;
  tag: string;
  color: string;
  border: string;
  points: string[];
}

interface Step {
  waypoint: string;
  title: string;
  body: string;
}

interface Integration {
  label: string;
  desc: string;
  mono: string;
}

interface PainPoint {
  headline: string;
  body: string;
}

interface DeploymentTier {
  tier: string;
  tag: string;
  desc: string;
}

type EmailStatus = "idle" | "success" | "error";
type CaptureSize = "default" | "large";

// ─── Icons ────────────────────────────────────────────────────────────────────

const RadarIcon = (): ReactNode => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    className="w-5 h-5"
  >
    <circle cx="12" cy="12" r="10" />
    <circle cx="12" cy="12" r="6" />
    <circle cx="12" cy="12" r="2" />
    <line x1="12" y1="2" x2="12" y2="12" strokeLinecap="round" />
  </svg>
);

const RouteIcon = (): ReactNode => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    className="w-5 h-5"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      d="M3 7h4l2 4h8l2-4h2M7 7l-1 8h12l-1-8"
    />
    <circle cx="9" cy="19" r="1.5" />
    <circle cx="15" cy="19" r="1.5" />
  </svg>
);

const ShieldIcon = (): ReactNode => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    className="w-5 h-5"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      d="M12 2l7 4v6c0 5-3.5 8.5-7 10C8.5 20.5 5 17 5 12V6l7-4z"
    />
    <path strokeLinecap="round" d="M9 12l2 2 4-4" />
  </svg>
);

const LockIcon = (): ReactNode => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    className="w-5 h-5"
  >
    <rect x="3" y="11" width="18" height="11" rx="2" />
    <path strokeLinecap="round" d="M7 11V7a5 5 0 0110 0v4" />
  </svg>
);

const CheckIcon = (): ReactNode => (
  <svg
    viewBox="0 0 20 20"
    fill="currentColor"
    className="w-4 h-4 text-cyan-400 flex-shrink-0 mt-0.5"
  >
    <path
      fillRule="evenodd"
      d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
      clipRule="evenodd"
    />
  </svg>
);

const ArrowRight = (): ReactNode => (
  <svg viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
    <path
      fillRule="evenodd"
      d="M10.293 5.293a1 1 0 011.414 0l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414-1.414L12.586 11H5a1 1 0 110-2h7.586l-2.293-2.293a1 1 0 010-1.414z"
      clipRule="evenodd"
    />
  </svg>
);

// ─── Data ─────────────────────────────────────────────────────────────────────

const features: Feature[] = [
  {
    icon: <RadarIcon />,
    title: "Observability",
    tag: "BLACK BOX RECORDER",
    color: "text-cyan-400",
    border: "border-cyan-900/50 hover:border-cyan-700/60",
    points: [
      "Thread → Trace → Span hierarchy, OTel-native",
      "Token count, TTFT, TPS, and cost per call",
      "Export to Jaeger, Datadog, Grafana, or your own collector",
      "Durable span buffer for long-running async agents",
    ],
  },
  {
    icon: <RouteIcon />,
    title: "Intelligent Routing",
    tag: "AIR TRAFFIC CONTROL",
    color: "text-sky-400",
    border: "border-sky-900/50 hover:border-sky-700/60",
    points: [
      "Circuit breaker: Closed → Open → Half-Open",
      "Signals: TTFT P99, TPS, error rate, quota consumption",
      "Scoring weights configurable per agent type",
      "Cross-key deprioritisation when quota nears limit",
    ],
  },
  {
    icon: <ShieldIcon />,
    title: "Guardrails Engine",
    tag: "CONTROLLED AIRSPACE",
    color: "text-amber-400",
    border: "border-amber-900/50 hover:border-amber-700/60",
    points: [
      "Priority-ordered rules: block, rewrite, tag, shadow",
      "Parallel evaluation — lowest latency wins",
      "Managed rules: injection, toxicity, topic restriction",
      "Custom rules via sandboxed WASM / Deno scripts",
    ],
  },
  {
    icon: <LockIcon />,
    title: "PII Masking",
    tag: "CARGO MANIFEST",
    color: "text-emerald-400",
    border: "border-emerald-900/50 hover:border-emerald-700/60",
    points: [
      "Regex + NER hybrid detection pipeline",
      "Inbound masking → token vault → LLM",
      "Outbound: tokens dereferenced before the user sees them",
      "BYO KMS — vault keys never held by the gateway",
    ],
  },
];

const steps: Step[] = [
  {
    waypoint: "W01",
    title: "File your flight plan",
    body: "Point base_url at the gateway. One line. Traces start flowing immediately — no SDK rewrite, no agent logic to change.",
  },
  {
    waypoint: "W02",
    title: "Define controlled airspace",
    body: "Write guardrail rules with priority, scope, and action. Managed rules ship on day one. Add WASM scripts for nuanced cases.",
  },
  {
    waypoint: "W03",
    title: "Seal the cargo manifest",
    body: "Select PII entity types, set vault TTL, wire in your KMS. Sensitive data is tokenised before it ever reaches the model.",
  },
  {
    waypoint: "W04",
    title: "Monitor from the tower",
    body: "Live cost dashboards, TTFT histograms, guardrail fire rates, and circuit breaker state — full situational awareness.",
  },
];

const integrations: Integration[] = [
  {
    label: "OpenAI SDK",
    mono: "base_url →",
    desc: "Zero code changes. Your app doesn't know there's a gateway.",
  },
  {
    label: "LangChain / LangGraph",
    mono: "SDK",
    desc: "First-class span primitives and tool-call interception.",
  },
  {
    label: "Google ADK",
    mono: "SDK",
    desc: "Agent graph structure and tool schemas flow natively.",
  },
  {
    label: "Self-hosted",
    mono: "Helm / Compose",
    desc: "BYO KMS. OTel to your own collector. Data stays in your VPC.",
  },
];

const painPoints: PainPoint[] = [
  {
    headline: "Your token bill doubled overnight and you have no idea why.",
    body: "No per-agent cost breakdown. No trace to diff against. Just a number on an invoice and a guess.",
  },
  {
    headline:
      "A user jailbroke your agent. You found out from a support ticket.",
    body: "No controlled airspace, no block rule, no audit trail. It happened in plain sight and nothing fired.",
  },
  {
    headline: "OpenAI had an outage. Your product went down with it.",
    body: "No alternate route filed. One runway closed and your entire fleet was grounded with it.",
  },
];

const deploymentTiers: DeploymentTier[] = [
  {
    tier: "SaaS",
    tag: "SHARED",
    desc: "Start immediately. Logical tenant isolation. US, EU, APAC regions.",
  },
  {
    tier: "Dedicated SaaS",
    tag: "MID-MARKET",
    desc: "Dedicated infrastructure, same control plane. No noisy neighbours.",
  },
  {
    tier: "Self-hosted",
    tag: "ENTERPRISE",
    desc: "Helm + Docker Compose. BYO KMS. Vault data never leaves your VPC.",
  },
];

// ─── Radar Sweep ──────────────────────────────────────────────────────────────

const RadarSweep = (): ReactNode => (
  <>
    <style>{`
      @keyframes radar-sweep {
        from { transform: rotate(0deg); }
        to   { transform: rotate(360deg); }
      }
      @keyframes radar-ping {
        0%   { opacity: 0.8; transform: scale(1); }
        100% { opacity: 0;   transform: scale(1.6); }
      }
      .radar-arm {
        animation: radar-sweep 4s linear infinite;
        transform-origin: center center;
      }
      .radar-blip {
        animation: radar-ping 4s ease-out infinite;
      }
      .radar-blip-2 {
        animation: radar-ping 4s ease-out 1.3s infinite;
      }
      .radar-blip-3 {
        animation: radar-ping 4s ease-out 2.6s infinite;
      }
    `}</style>
    <svg
      viewBox="0 0 400 400"
      className="absolute inset-0 w-full h-full opacity-20"
      aria-hidden="true"
    >
      {/* Ring grid */}
      {[40, 80, 120, 160, 200].map((r) => (
        <circle
          key={r}
          cx="200"
          cy="200"
          r={r}
          fill="none"
          stroke="#22d3ee"
          strokeWidth="0.5"
        />
      ))}
      {/* Cross hairs */}
      <line
        x1="200"
        y1="0"
        x2="200"
        y2="400"
        stroke="#22d3ee"
        strokeWidth="0.5"
      />
      <line
        x1="0"
        y1="200"
        x2="400"
        y2="200"
        stroke="#22d3ee"
        strokeWidth="0.5"
      />
      {/* Sweep arm */}
      <g className="radar-arm">
        <line
          x1="200"
          y1="200"
          x2="200"
          y2="40"
          stroke="#22d3ee"
          strokeWidth="1.5"
          strokeLinecap="round"
        />
        <path
          d="M200 200 L200 40 A160 160 0 0 1 360 200 Z"
          fill="url(#sweep-grad)"
        />
      </g>
      <defs>
        <radialGradient id="sweep-grad" cx="50%" cy="50%" r="50%">
          <stop offset="0%" stopColor="#22d3ee" stopOpacity="0" />
          <stop offset="100%" stopColor="#22d3ee" stopOpacity="0.15" />
        </radialGradient>
      </defs>
      {/* Blips */}
      <circle className="radar-blip" cx="280" cy="130" r="6" fill="#22d3ee" />
      <circle className="radar-blip-2" cx="140" cy="260" r="5" fill="#22d3ee" />
      <circle className="radar-blip-3" cx="310" cy="270" r="4" fill="#22d3ee" />
    </svg>
  </>
);

// ─── Flight Path Topology (hero visual) ──────────────────────────────────────

const FlightPathDiagram = (): ReactNode => (
  <>
    <style>{`
      @keyframes fly {
        0%   { offset-distance: 0%;   opacity: 0; }
        10%  { opacity: 1; }
        90%  { opacity: 1; }
        100% { offset-distance: 100%; opacity: 0; }
      }
      .dot-fly { animation: fly 2.4s ease-in-out infinite; }
      .dot-fly-2 { animation: fly 2.4s ease-in-out 0.6s infinite; }
      .dot-fly-3 { animation: fly 2.4s ease-in-out 1.2s infinite; }
      .dot-fly-out   { animation: fly 2.8s ease-in-out 0.4s infinite; }
      .dot-fly-out-2 { animation: fly 2.8s ease-in-out 1.1s infinite; }
      .dot-fly-out-3 { animation: fly 2.8s ease-in-out 1.8s infinite; }
    `}</style>

    {/* ── Horizontal Version (Desktop/Tablet) ── */}
    <div className="relative w-full mt-20 px-0 overflow-x-auto hidden sm:block">
      <div className="min-w-[600px]">
        <svg viewBox="0 0 700 260" className="w-full" aria-hidden="true">
          {/* ── Agent nodes (left) ── */}
          {[
            { y: 60, label: "AGT-001", sub: "customer-support" },
            { y: 130, label: "AGT-002", sub: "data-pipeline" },
            { y: 200, label: "AGT-003", sub: "code-assistant" },
          ].map(({ y, label, sub }) => (
            <g key={label}>
              <rect
                x="8"
                y={y - 22}
                width="140"
                height="44"
                rx="6"
                fill="#0d1520"
                stroke="#164e63"
                strokeWidth="1"
              />
              <text
                x="78"
                y={y - 5}
                textAnchor="middle"
                fill="#22d3ee"
                fontSize="10"
                fontFamily="monospace"
                fontWeight="700"
              >
                {label}
              </text>
              <text
                x="78"
                y={y + 11}
                textAnchor="middle"
                fill="#4b6475"
                fontSize="9"
                fontFamily="monospace"
              >
                {sub}
              </text>
            </g>
          ))}

          {/* ── Inbound flight paths ── */}
          {[
            {
              id: "p1",
              d: "M 148 60  C 260 60  260 130 310 130",
              cls: "dot-fly",
              delay: "0s",
            },
            {
              id: "p2",
              d: "M 148 130 C 240 130 270 130 310 130",
              cls: "dot-fly-2",
              delay: "0.6s",
            },
            {
              id: "p3",
              d: "M 148 200 C 260 200 260 130 310 130",
              cls: "dot-fly-3",
              delay: "1.2s",
            },
          ].map(({ id, d, cls }) => (
            <g key={id}>
              <path
                id={id}
                d={d}
                fill="none"
                stroke="#164e63"
                strokeWidth="1"
                strokeDasharray="4 4"
              />
              <circle
                r="4"
                fill="#22d3ee"
                style={{ offsetPath: `path('${d}')` }}
                className={cls}
              />
            </g>
          ))}

          {/* ── Gateway (center) ── */}
          <rect
            x="285"
            y="98"
            width="130"
            height="64"
            rx="8"
            fill="#071820"
            stroke="#22d3ee"
            strokeWidth="1.5"
          />
          <text
            x="350"
            y="123"
            textAnchor="middle"
            fill="#22d3ee"
            fontSize="11"
            fontFamily="monospace"
            fontWeight="800"
            letterSpacing="1"
          >
            AI GATEWAY
          </text>
          <circle cx="310" cy="143" r="3" fill="#22d3ee" opacity="0.9" />
          <circle cx="322" cy="143" r="3" fill="#22d3ee" opacity="0.6" />
          <circle cx="334" cy="143" r="3" fill="#22d3ee" opacity="0.3" />
          <text
            x="388"
            y="147"
            fill="#4b6475"
            fontSize="9"
            fontFamily="monospace"
          >
            TOWER
          </text>

          {/* ── Outbound flight paths ── */}
          {[
            {
              id: "o1",
              d: "M 415 120 C 470 120 510 70  552 70",
              cls: "dot-fly-out",
            },
            {
              id: "o2",
              d: "M 415 130 C 480 130 510 130 552 130",
              cls: "dot-fly-out-2",
            },
            {
              id: "o3",
              d: "M 415 140 C 470 140 510 190 552 190",
              cls: "dot-fly-out-3",
            },
          ].map(({ id, d, cls }) => (
            <g key={id}>
              <path
                id={id}
                d={d}
                fill="none"
                stroke="#164e63"
                strokeWidth="1"
                strokeDasharray="4 4"
              />
              <circle
                r="4"
                fill="#67e8f9"
                style={{ offsetPath: `path('${d}')` }}
                className={cls}
              />
            </g>
          ))}

          {/* ── LLM Provider nodes (right) ── */}
          {[
            { y: 70, label: "GPT-4o", status: "#22d3ee" },
            { y: 130, label: "Claude 3.5", status: "#22d3ee" },
            { y: 190, label: "Gemini Pro", status: "#f59e0b" },
          ].map(({ y, label, status }) => (
            <g key={label}>
              <rect
                x="552"
                y={y - 22}
                width="130"
                height="44"
                rx="6"
                fill="#0d1520"
                stroke="#164e63"
                strokeWidth="1"
              />
              <circle cx="572" cy={y} r="4" fill={status} />
              <text
                x="638"
                y={y + 5}
                textAnchor="middle"
                fill="#94a3b8"
                fontSize="10"
                fontFamily="monospace"
              >
                {label}
              </text>
            </g>
          ))}

          {/* ── Status bar ── */}
          <rect x="8" y="242" width="684" height="1" fill="#164e63" />
          {[
            { x: 8, label: "TTFT P99", value: "124ms", color: "#22d3ee" },
            { x: 180, label: "COST/HR", value: "$0.031", color: "#22d3ee" },
            {
              x: 340,
              label: "GUARDRAILS",
              value: "12 FIRED",
              color: "#f59e0b",
            },
            { x: 510, label: "CIRCUIT", value: "CLOSED", color: "#4ade80" },
          ].map(({ x, label, value, color }) => (
            <g key={label}>
              <text
                x={x}
                y="256"
                fill="#4b6475"
                fontSize="8"
                fontFamily="monospace"
              >
                {label}
              </text>
              <text
                x={x}
                y="268"
                fill={color}
                fontSize="9"
                fontFamily="monospace"
                fontWeight="700"
              >
                {value}
              </text>
            </g>
          ))}
        </svg>
      </div>
    </div>

    {/* ── Vertical Version (Mobile) ── */}
    <div className="relative w-full mt-12 px-0 block sm:hidden">
      <svg viewBox="0 0 320 440" className="w-full" aria-hidden="true">
        {/* ── Agent nodes (top) ── */}
        {[
          { x: 10, label: "AGT-001", sub: "support" },
          { x: 115, label: "AGT-002", sub: "pipeline" },
          { x: 220, label: "AGT-003", sub: "code" },
        ].map(({ x, label, sub }) => (
          <g key={label}>
            <rect
              x={x}
              y="20"
              width="90"
              height="44"
              rx="6"
              fill="#0d1520"
              stroke="#164e63"
              strokeWidth="1"
            />
            <text
              x={x + 45}
              y="37"
              textAnchor="middle"
              fill="#22d3ee"
              fontSize="10"
              fontFamily="monospace"
              fontWeight="700"
            >
              {label}
            </text>
            <text
              x={x + 45}
              y="53"
              textAnchor="middle"
              fill="#4b6475"
              fontSize="9"
              fontFamily="monospace"
            >
              {sub}
            </text>
          </g>
        ))}

        {/* ── Inbound flight paths ── */}
        {[
          {
            id: "vp1",
            d: "M 55 64 C 55 100 160 100 160 140",
            cls: "dot-fly",
            delay: "0s",
          },
          {
            id: "vp2",
            d: "M 160 64 C 160 100 160 100 160 140",
            cls: "dot-fly-2",
            delay: "0.6s",
          },
          {
            id: "vp3",
            d: "M 265 64 C 265 100 160 100 160 140",
            cls: "dot-fly-3",
            delay: "1.2s",
          },
        ].map(({ id, d, cls }) => (
          <g key={id}>
            <path
              id={id}
              d={d}
              fill="none"
              stroke="#164e63"
              strokeWidth="1"
              strokeDasharray="4 4"
            />
            <circle
              r="4"
              fill="#22d3ee"
              style={{ offsetPath: `path('${d}')` }}
              className={cls}
            />
          </g>
        ))}

        {/* ── Gateway (center) ── */}
        <rect
          x="95"
          y="140"
          width="130"
          height="64"
          rx="8"
          fill="#071820"
          stroke="#22d3ee"
          strokeWidth="1.5"
        />
        <text
          x="160"
          y="165"
          textAnchor="middle"
          fill="#22d3ee"
          fontSize="11"
          fontFamily="monospace"
          fontWeight="800"
          letterSpacing="1"
        >
          AI GATEWAY
        </text>
        <circle cx="120" cy="185" r="3" fill="#22d3ee" opacity="0.9" />
        <circle cx="132" cy="185" r="3" fill="#22d3ee" opacity="0.6" />
        <circle cx="144" cy="185" r="3" fill="#22d3ee" opacity="0.3" />
        <text
          x="175"
          y="189"
          fill="#4b6475"
          fontSize="9"
          fontFamily="monospace"
        >
          TOWER
        </text>

        {/* ── Outbound flight paths ── */}
        {[
          {
            id: "vo1",
            d: "M 160 204 C 160 240 55 240 55 260",
            cls: "dot-fly-out",
          },
          {
            id: "vo2",
            d: "M 160 204 C 160 240 160 240 160 260",
            cls: "dot-fly-out-2",
          },
          {
            id: "vo3",
            d: "M 160 204 C 160 240 265 240 265 260",
            cls: "dot-fly-out-3",
          },
        ].map(({ id, d, cls }) => (
          <g key={id}>
            <path
              id={id}
              d={d}
              fill="none"
              stroke="#164e63"
              strokeWidth="1"
              strokeDasharray="4 4"
            />
            <circle
              r="4"
              fill="#67e8f9"
              style={{ offsetPath: `path('${d}')` }}
              className={cls}
            />
          </g>
        ))}

        {/* ── LLM Provider nodes (bottom) ── */}
        {[
          { x: 10, label: "GPT-4o", status: "#22d3ee" },
          { x: 115, label: "Claude", status: "#22d3ee" },
          { x: 220, label: "Gemini", status: "#f59e0b" },
        ].map(({ x, label, status }) => (
          <g key={label}>
            <rect
              x={x}
              y="260"
              width="90"
              height="44"
              rx="6"
              fill="#0d1520"
              stroke="#164e63"
              strokeWidth="1"
            />
            <circle cx={x + 15} cy="282" r="4" fill={status} />
            <text
              x={x + 48}
              y="286"
              textAnchor="middle"
              fill="#94a3b8"
              fontSize="10"
              fontFamily="monospace"
            >
              {label}
            </text>
          </g>
        ))}

        {/* ── Status bar ── */}
        <rect x="10" y="340" width="300" height="1" fill="#164e63" />
        {[
          {
            x: 10,
            y: 360,
            label: "TTFT P99",
            value: "124ms",
            color: "#22d3ee",
          },
          {
            x: 170,
            y: 360,
            label: "COST/HR",
            value: "$0.031",
            color: "#22d3ee",
          },
          {
            x: 10,
            y: 400,
            label: "GUARDRAILS",
            value: "12 FIRED",
            color: "#f59e0b",
          },
          {
            x: 170,
            y: 400,
            label: "CIRCUIT",
            value: "CLOSED",
            color: "#4ade80",
          },
        ].map(({ x, y, label, value, color }) => (
          <g key={label}>
            <text
              x={x}
              y={y}
              fill="#4b6475"
              fontSize="8"
              fontFamily="monospace"
            >
              {label}
            </text>
            <text
              x={x}
              y={y + 16}
              fill={color}
              fontSize="11"
              fontFamily="monospace"
              fontWeight="700"
            >
              {value}
            </text>
          </g>
        ))}
      </svg>
    </div>
  </>
);

// ─── Email Capture ────────────────────────────────────────────────────────────

interface EmailCaptureProps {
  size?: CaptureSize;
}

function EmailCapture({ size = "default" }: EmailCaptureProps): ReactNode {
  const [email, setEmail] = useState<string>("");
  const [status, setStatus] = useState<EmailStatus>("idle");

  const handleSubmit = (e: FormEvent<HTMLFormElement>): void => {
    e.preventDefault();
    if (!email || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      setStatus("error");
      return;
    }
    setStatus("success");
    setEmail("");
  };

  if (status === "success") {
    return (
      <div className="flex items-center gap-3 text-cyan-400 font-mono text-sm">
        <CheckIcon />
        <span>Cleared for early access. We'll be in touch.</span>
      </div>
    );
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="flex flex-col sm:flex-row gap-3 w-full max-w-md"
    >
      <div className="flex-1">
        <input
          type="email"
          value={email}
          onChange={(e) => {
            setEmail(e.target.value);
            setStatus("idle");
          }}
          placeholder="your@email.com"
          className={`w-full bg-cyan-950/20 border ${
            status === "error" ? "border-red-500" : "border-cyan-900/60"
          } rounded-lg px-4 ${
            size === "large" ? "py-4 text-base" : "py-3 text-sm"
          } text-white placeholder-cyan-900 focus:outline-none focus:border-cyan-500 focus:ring-1 focus:ring-cyan-500/50 transition-colors font-mono`}
        />
        {status === "error" && (
          <p className="mt-1 text-xs text-red-400 font-mono">
            Invalid callsign — check your email address.
          </p>
        )}
      </div>
      <button
        type="submit"
        className={`flex items-center justify-center gap-2 bg-cyan-500 hover:bg-cyan-400 active:bg-cyan-600 text-gray-950 font-bold rounded-lg px-6 ${
          size === "large" ? "py-4 text-base" : "py-3 text-sm"
        } transition-colors whitespace-nowrap tracking-wide`}
      >
        Request access <ArrowRight />
      </button>
    </form>
  );
}

// ─── Dashboard Mockup ────────────────────────────────────────────────────────

const chartData = [
  0.21, 0.16, 0.11, 0.08, 0.05, 0.08, 0.21, 0.37, 0.55, 0.74, 0.84, 0.79, 0.76,
  0.82, 0.71, 0.63, 0.74, 0.92, 1.0, 0.84, 0.76, 0.63, 0.47, 0.32,
];
const W = 460;
const H = 72;
const pts = chartData.map(
  (v, i) => [i * (W / (chartData.length - 1)), (1 - v) * H] as [number, number],
);
const smooth = pts
  .map(([x, y], i) => {
    if (i === 0) return `M ${x} ${y}`;
    const [px, py] = pts[i - 1];
    const cx1 = px + (x - px) / 3;
    const cx2 = x - (x - px) / 3;
    return `C ${cx1} ${py} ${cx2} ${y} ${x} ${y}`;
  })
  .join(" ");
const areaPath = `${smooth} L ${W} ${H} L 0 ${H} Z`;

const traces = [
  {
    agent: "AGT-001",
    sub: "customer-support",
    model: "gpt-4o",
    tokens: "1,247",
    cost: "$0.031",
    ttft: "89ms",
    status: "OK",
    statusColor: "#22d3ee",
  },
  {
    agent: "AGT-002",
    sub: "data-pipeline",
    model: "claude-3.5",
    tokens: "8,430",
    cost: "$0.021",
    ttft: "234ms",
    status: "GUARDRAIL",
    statusColor: "#f59e0b",
  },
  {
    agent: "AGT-001",
    sub: "customer-support",
    model: "gpt-4o",
    tokens: "2,103",
    cost: "$0.052",
    ttft: "112ms",
    status: "PII MASKED",
    statusColor: "#a78bfa",
  },
  {
    agent: "AGT-003",
    sub: "code-assistant",
    model: "gpt-4o",
    tokens: "891",
    cost: "$0.022",
    ttft: "156ms",
    status: "OK",
    statusColor: "#22d3ee",
  },
  {
    agent: "AGT-002",
    sub: "data-pipeline",
    model: "gemini-pro",
    tokens: "3,812",
    cost: "$0.009",
    ttft: "310ms",
    status: "FALLBACK",
    statusColor: "#f59e0b",
  },
];

const breakers = [
  { label: "GPT-4o", state: "CLOSED", color: "#4ade80", sub: "p99 · 124ms" },
  {
    label: "Claude 3.5",
    state: "CLOSED",
    color: "#4ade80",
    sub: "p99 · 201ms",
  },
  {
    label: "Gemini Pro",
    state: "HALF-OPEN",
    color: "#f59e0b",
    sub: "probe in 12s",
  },
];

function DashboardMockup(): ReactNode {
  return (
    <div
      className="rounded-xl overflow-hidden shadow-2xl w-full"
      style={{
        background: "#050d14",
        border: "1px solid rgba(34,211,238,0.18)",
      }}
    >
      {/* Browser chrome */}
      <div
        className="flex items-center gap-2 px-4 py-3 border-b"
        style={{
          background: "rgba(34,211,238,0.04)",
          borderColor: "rgba(34,211,238,0.1)",
        }}
      >
        <div className="w-2.5 h-2.5 rounded-full bg-red-500/60" />
        <div className="w-2.5 h-2.5 rounded-full bg-yellow-500/60" />
        <div className="w-2.5 h-2.5 rounded-full bg-green-500/60" />
        <div
          className="ml-4 flex-1 max-w-xs rounded px-3 py-1 text-xs font-mono"
          style={{
            background: "rgba(34,211,238,0.05)",
            color: "#4b6475",
            border: "1px solid rgba(34,211,238,0.1)",
          }}
        >
          gateway.theslmcompany.com/dashboard
        </div>
        <span
          className="ml-auto text-xs font-mono px-2 py-0.5 rounded flex items-center gap-1.5"
          style={{ background: "rgba(34,211,238,0.08)", color: "#22d3ee" }}
        >
          <span className="w-1.5 h-1.5 rounded-full bg-cyan-400 animate-pulse" />
          LIVE
        </span>
      </div>

      {/* Dashboard top nav */}
      <div
        className="flex items-center gap-6 px-5 py-2.5 border-b"
        style={{
          borderColor: "rgba(34,211,238,0.07)",
          background: "rgba(7,12,18,0.9)",
        }}
      >
        <span
          className="font-mono text-xs font-bold tracking-widest"
          style={{ color: "#22d3ee" }}
        >
          AI GATEWAY
        </span>
        {["Overview", "Traces", "Guardrails", "PII Vault", "Router"].map(
          (item, i) => (
            <span
              key={item}
              className="text-xs font-mono cursor-default"
              style={{
                color: i === 0 ? "#22d3ee" : "#2d4a54",
                borderBottom: i === 0 ? "1px solid #22d3ee" : "none",
                paddingBottom: i === 0 ? "2px" : undefined,
              }}
            >
              {item}
            </span>
          ),
        )}
        <div className="ml-auto flex items-center gap-3">
          <span className="text-xs font-mono" style={{ color: "#2d4a54" }}>
            Last 24h ▾
          </span>
          <div className="w-6 h-6 rounded-full bg-cyan-900/40 flex items-center justify-center">
            <span className="text-xs font-mono" style={{ color: "#22d3ee" }}>
              C
            </span>
          </div>
        </div>
      </div>

      {/* Dashboard body */}
      <div className="p-5 space-y-5">
        {/* Stat cards */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          {[
            {
              label: "TTFT P99",
              value: "124ms",
              delta: "↓ 12%",
              good: true,
              sub: "vs. yesterday",
            },
            {
              label: "COST / 24H",
              value: "$2.41",
              delta: "↑ 8%",
              good: false,
              sub: "3 agents",
            },
            {
              label: "GUARDRAIL FIRES",
              value: "47",
              delta: "↑ 3",
              good: false,
              sub: "this hour",
            },
            {
              label: "AGENTS ONLINE",
              value: "3",
              delta: "stable",
              good: true,
              sub: "all healthy",
            },
          ].map(({ label, value, delta, good, sub }) => (
            <div
              key={label}
              className="rounded-lg p-3"
              style={{
                background: "rgba(13,21,32,0.9)",
                border: "1px solid rgba(34,211,238,0.08)",
              }}
            >
              <p
                className="text-xs font-mono font-bold tracking-widest mb-2"
                style={{ color: "#2d4a54" }}
              >
                {label}
              </p>
              <p className="text-xl font-black font-mono text-white mb-1">
                {value}
              </p>
              <div className="flex items-center gap-1.5">
                <span
                  className="text-xs font-mono"
                  style={{ color: good ? "#4ade80" : "#f59e0b" }}
                >
                  {delta}
                </span>
                <span
                  className="text-xs font-mono"
                  style={{ color: "#1e3340" }}
                >
                  {sub}
                </span>
              </div>
            </div>
          ))}
        </div>

        {/* Chart + circuit breakers row */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-3">
          {/* Cost chart */}
          <div
            className="col-span-1 lg:col-span-2 rounded-lg p-4"
            style={{
              background: "rgba(13,21,32,0.9)",
              border: "1px solid rgba(34,211,238,0.08)",
            }}
          >
            <div className="flex items-center justify-between mb-4">
              <div>
                <p
                  className="text-xs font-mono font-bold tracking-widest mb-0.5"
                  style={{ color: "#2d4a54" }}
                >
                  COST / HOUR
                </p>
                <p className="text-sm font-mono font-bold text-white">
                  $0.038{" "}
                  <span
                    className="text-xs font-normal"
                    style={{ color: "#2d4a54" }}
                  >
                    current hour
                  </span>
                </p>
              </div>
              <div className="flex gap-3">
                {[
                  { label: "gpt-4o", color: "#22d3ee" },
                  { label: "claude-3.5", color: "#a78bfa" },
                  { label: "gemini", color: "#f59e0b" },
                ].map(({ label, color }) => (
                  <div key={label} className="flex items-center gap-1">
                    <div
                      className="w-2 h-2 rounded-full"
                      style={{ background: color }}
                    />
                    <span
                      className="text-xs font-mono"
                      style={{ color: "#2d4a54" }}
                    >
                      {label}
                    </span>
                  </div>
                ))}
              </div>
            </div>
            <svg
              viewBox={`0 0 ${W} ${H + 20}`}
              className="w-full"
              preserveAspectRatio="none"
              style={{ height: "72px" }}
            >
              <defs>
                <linearGradient id="areaGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#22d3ee" stopOpacity="0.15" />
                  <stop offset="100%" stopColor="#22d3ee" stopOpacity="0" />
                </linearGradient>
              </defs>
              {/* Grid lines */}
              {[0, 0.25, 0.5, 0.75, 1].map((v) => (
                <line
                  key={v}
                  x1="0"
                  y1={v * H}
                  x2={W}
                  y2={v * H}
                  stroke="rgba(34,211,238,0.05)"
                  strokeWidth="1"
                />
              ))}
              {/* Area fill */}
              <path d={areaPath} fill="url(#areaGrad)" />
              {/* Line */}
              <path d={smooth} fill="none" stroke="#22d3ee" strokeWidth="1.5" />
              {/* Current value dot */}
              <circle
                cx={pts[pts.length - 1][0]}
                cy={pts[pts.length - 1][1]}
                r="3"
                fill="#22d3ee"
              />
              {/* Hour labels */}
              {[0, 6, 12, 18, 23].map((h) => (
                <text
                  key={h}
                  x={h * (W / 23)}
                  y={H + 14}
                  textAnchor="middle"
                  fill="#1e3340"
                  fontSize="8"
                  fontFamily="monospace"
                >
                  {h === 0
                    ? "00:00"
                    : h === 23
                      ? "now"
                      : `${String(h).padStart(2, "0")}:00`}
                </text>
              ))}
            </svg>
          </div>

          {/* Circuit breakers */}
          <div
            className="rounded-lg p-4"
            style={{
              background: "rgba(13,21,32,0.9)",
              border: "1px solid rgba(34,211,238,0.08)",
            }}
          >
            <p
              className="text-xs font-mono font-bold tracking-widest mb-4"
              style={{ color: "#2d4a54" }}
            >
              CIRCUIT BREAKERS
            </p>
            <div className="space-y-3">
              {breakers.map(({ label, state, color, sub }) => (
                <div
                  key={label}
                  className="flex items-center justify-between rounded px-3 py-2.5"
                  style={{
                    background: "rgba(7,12,18,0.8)",
                    border: `1px solid ${color}22`,
                  }}
                >
                  <div>
                    <p className="text-xs font-mono font-bold text-white">
                      {label}
                    </p>
                    <p
                      className="text-xs font-mono mt-0.5"
                      style={{ color: "#1e3340" }}
                    >
                      {sub}
                    </p>
                  </div>
                  <span
                    className="text-xs font-mono font-bold"
                    style={{ color }}
                  >
                    {state}
                  </span>
                </div>
              ))}
              <div className="pt-1">
                <p className="text-xs font-mono" style={{ color: "#1e3340" }}>
                  Fallback routing active for{" "}
                  <span style={{ color: "#f59e0b" }}>Gemini Pro</span>
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* Traces table */}
        <div
          className="rounded-lg overflow-hidden"
          style={{
            background: "rgba(13,21,32,0.9)",
            border: "1px solid rgba(34,211,238,0.08)",
          }}
        >
          <div
            className="flex items-center justify-between px-4 py-3 border-b"
            style={{ borderColor: "rgba(34,211,238,0.07)" }}
          >
            <p
              className="text-xs font-mono font-bold tracking-widest"
              style={{ color: "#2d4a54" }}
            >
              RECENT TRACES
            </p>
            <span className="text-xs font-mono" style={{ color: "#1e3340" }}>
              showing 5 of 1,847
            </span>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs font-mono min-w-[600px]">
              <thead>
                <tr style={{ borderBottom: "1px solid rgba(34,211,238,0.07)" }}>
                  {["AGENT", "MODEL", "TOKENS", "COST", "TTFT", "STATUS"].map(
                    (h) => (
                      <th
                        key={h}
                        className="text-left px-4 py-2 font-bold tracking-widest"
                        style={{ color: "#1e3340" }}
                      >
                        {h}
                      </th>
                    ),
                  )}
                </tr>
              </thead>
              <tbody>
                {traces.map((t, i) => (
                  <tr
                    key={i}
                    style={{
                      borderBottom:
                        i < traces.length - 1
                          ? "1px solid rgba(34,211,238,0.04)"
                          : undefined,
                    }}
                  >
                    <td className="px-4 py-2">
                      <span className="text-white">{t.agent}</span>
                      <span className="ml-2" style={{ color: "#1e3340" }}>
                        {t.sub}
                      </span>
                    </td>
                    <td className="px-4 py-2" style={{ color: "#2d4a54" }}>
                      {t.model}
                    </td>
                    <td className="px-4 py-2" style={{ color: "#2d4a54" }}>
                      {t.tokens}
                    </td>
                    <td className="px-4 py-2" style={{ color: "#2d4a54" }}>
                      {t.cost}
                    </td>
                    <td className="px-4 py-2" style={{ color: "#2d4a54" }}>
                      {t.ttft}
                    </td>
                    <td className="px-4 py-2">
                      <span
                        className="px-2 py-0.5 rounded text-xs font-bold whitespace-nowrap"
                        style={{
                          background: `${t.statusColor}14`,
                          color: t.statusColor,
                          border: `1px solid ${t.statusColor}33`,
                        }}
                      >
                        {t.status}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}

// ─── Main Page ────────────────────────────────────────────────────────────────

export default function AIGatewayLanding(): ReactNode {
  return (
    <div
      className="min-h-screen text-white antialiased"
      style={{
        backgroundColor: "#070c12",
        fontFamily: "system-ui, sans-serif",
      }}
    >
      {/* Global grid background */}
      <div
        className="fixed inset-0 pointer-events-none"
        style={{
          backgroundImage:
            "linear-gradient(rgba(34,211,238,0.03) 1px, transparent 1px), linear-gradient(90deg, rgba(34,211,238,0.03) 1px, transparent 1px)",
          backgroundSize: "48px 48px",
        }}
      />

      {/* ── Nav ── */}
      <nav
        className="sticky top-0 z-50 border-b"
        style={{
          backgroundColor: "rgba(7,12,18,0.85)",
          borderColor: "rgba(34,211,238,0.1)",
          backdropFilter: "blur(12px)",
        }}
      >
        <div className="w-full px-6 lg:px-12 xl:px-24 h-14 flex items-center justify-between">
          <Link
            to="/"
            className="flex items-center gap-3"
            title="The SLM Company — home"
          >
            <div className="relative w-8 h-8">
              <div
                className="w-8 h-8 rounded-lg flex items-center justify-center transition-all hover:scale-105"
                style={{
                  background: "rgba(34,211,238,0.08)",
                  border: "1px solid rgba(34,211,238,0.2)",
                }}
              >
                <svg viewBox="0 0 32 32" fill="none" className="w-5 h-5">
                  <circle
                    cx="16"
                    cy="16"
                    r="14"
                    stroke="#22d3ee"
                    strokeWidth="1.5"
                    strokeOpacity="0.2"
                  />
                  <circle
                    cx="16"
                    cy="16"
                    r="8"
                    stroke="#22d3ee"
                    strokeWidth="1.5"
                    strokeOpacity="0.4"
                  />
                  <circle cx="16" cy="16" r="3" fill="#22d3ee" />
                  <path
                    d="M16 4V16L24 24"
                    stroke="#22d3ee"
                    strokeWidth="2"
                    strokeLinecap="round"
                  />
                </svg>
              </div>
            </div>
            <span className="font-mono font-black text-base tracking-[0.2em] text-white uppercase flex items-center gap-2">
              AI <span className="text-cyan-400">Gateway</span>
            </span>
          </Link>
          <div className="flex items-center gap-4">
            <span className="hidden sm:flex items-center gap-1.5 text-xs font-mono text-cyan-700">
              <span className="w-1.5 h-1.5 rounded-full bg-cyan-400 animate-pulse" />
              TOWER ONLINE
            </span>
            <a
              href="https://gateway.theslmcompany.com"
              className="text-xs font-mono font-bold tracking-widest px-4 py-2 rounded transition-colors hover:text-cyan-300"
              style={{ color: "#94a3b8" }}
            >
              LOGIN
            </a>
            <a
              href="#waitlist"
              className="text-xs font-mono font-bold tracking-widest px-4 py-2 rounded transition-colors"
              style={{
                background: "rgba(34,211,238,0.1)",
                border: "1px solid rgba(34,211,238,0.3)",
                color: "#22d3ee",
              }}
            >
              REQUEST ACCESS
            </a>
          </div>
        </div>
      </nav>

      {/* ── Hero ── */}
      <section className="relative overflow-hidden pt-20 pb-8 px-6 lg:px-12 xl:px-24">
        {/* Radar sweep, centered in hero */}
        <div className="absolute top-0 left-1/2 -translate-x-1/2 w-[520px] h-[520px] opacity-60 pointer-events-none">
          <RadarSweep />
        </div>

        <div className="relative max-w-4xl mx-auto text-center">
          <div
            className="inline-flex items-center gap-2 text-xs font-mono font-semibold px-4 py-1.5 rounded mb-10 tracking-widest uppercase"
            style={{
              background: "rgba(34,211,238,0.07)",
              border: "1px solid rgba(34,211,238,0.2)",
              color: "#67e8f9",
            }}
          >
            PRIVATE BETA · NOW BOARDING
          </div>

          <h1 className="text-4xl sm:text-5xl lg:text-[4.5rem] font-black tracking-tight leading-none mb-6">
            Air traffic control
            <br />
            <span style={{ color: "#22d3ee" }}>for your AI agents.</span>
          </h1>

          <p className="text-lg sm:text-xl text-slate-400 max-w-2xl mx-auto mb-3 leading-relaxed">
            A middleware gateway that sits between your agents and every LLM —
            routing traffic, enforcing guardrails, masking PII, and recording
            every span.
          </p>
          <p className="text-sm font-mono text-slate-600 mb-12">
            One URL change. No agent rewrites. Full situational awareness.
          </p>

          <div className="flex justify-center">
            <EmailCapture size="large" />
          </div>
          <p className="mt-4 text-xs font-mono text-slate-700">
            No spam. Early access notifications only.
          </p>
        </div>

        {/* Flight path diagram */}
        <FlightPathDiagram />

        {/* Code snippet — one line change */}
        <div className="relative w-full mt-10">
          <p className="text-center text-xs font-mono uppercase tracking-widest text-cyan-800 mb-4">
            One line to file your flight plan
          </p>
          <div
            className="rounded-xl overflow-hidden shadow-2xl"
            style={{
              background: "#050d14",
              border: "1px solid rgba(34,211,238,0.15)",
            }}
          >
            {/* Title bar */}
            <div
              className="flex items-center gap-1.5 px-4 py-3 border-b"
              style={{
                background: "rgba(34,211,238,0.04)",
                borderColor: "rgba(34,211,238,0.1)",
              }}
            >
              <div className="w-2.5 h-2.5 rounded-full bg-red-500/60" />
              <div className="w-2.5 h-2.5 rounded-full bg-yellow-500/60" />
              <div className="w-2.5 h-2.5 rounded-full bg-green-500/60" />
              <span className="ml-3 text-xs font-mono text-cyan-900">
                agent.py
              </span>
              <span
                className="ml-auto text-xs font-mono px-2 py-0.5 rounded"
                style={{
                  background: "rgba(34,211,238,0.08)",
                  color: "#22d3ee",
                }}
              >
                DIFF
              </span>
            </div>

            {/* Code body */}
            <div className="p-6 font-mono text-sm leading-relaxed space-y-1 overflow-x-auto whitespace-nowrap">
              {/* Removed line */}
              <div
                className="flex items-start gap-3 px-3 py-1.5 rounded"
                style={{
                  background: "rgba(220,38,38,0.08)",
                  borderLeft: "2px solid rgba(220,38,38,0.4)",
                }}
              >
                <span className="text-red-600 select-none flex-shrink-0">
                  −
                </span>
                <span className="text-slate-500 line-through">
                  client = OpenAI(base_url=
                  <span className="text-red-400/70">
                    "https://api.openai.com/v1"
                  </span>
                  )
                </span>
              </div>

              {/* Added line */}
              <div
                className="flex items-start gap-3 px-3 py-1.5 rounded"
                style={{
                  background: "rgba(34,211,238,0.06)",
                  borderLeft: "2px solid rgba(34,211,238,0.4)",
                }}
              >
                <span
                  style={{ color: "#22d3ee" }}
                  className="select-none shrink-0"
                >
                  +
                </span>
                <span className="text-slate-300">
                  client = OpenAI(base_url=
                  <span style={{ color: "#22d3ee" }}>
                    "https://gateway.theslmcompany.com/v1"
                  </span>
                  )
                </span>
              </div>

              {/* Unchanged context lines */}
              <div className="flex items-start gap-3 px-3 py-1 opacity-40">
                <span className="text-slate-600 select-none flex-shrink-0">
                  {" "}
                </span>
                <span className="text-slate-400">
                  &nbsp;&nbsp;extra_headers=&#123;
                  <span className="text-cyan-700">"X-Agent-Id"</span>: agent_id,{" "}
                  <span className="text-cyan-700">"X-Thread-Id"</span>:
                  session_id &#125;
                </span>
              </div>
            </div>

            {/* Footer bar */}
            <div
              className="flex items-center justify-between px-6 py-3 border-t"
              style={{
                borderColor: "rgba(34,211,238,0.08)",
                background: "rgba(34,211,238,0.02)",
              }}
            >
              <span className="text-xs font-mono text-cyan-900">
                Tracing · Guardrails · PII masking
              </span>
              <span
                className="text-xs font-mono px-2 py-0.5 rounded"
                style={{
                  background: "rgba(34,211,238,0.08)",
                  color: "#22d3ee",
                }}
              >
                ALL SYSTEMS ACTIVE
              </span>
            </div>
          </div>
        </div>
      </section>

      {/* ── Pain points ── */}
      <section
        className="border-y py-16 px-6 lg:px-12 xl:px-24"
        style={{
          borderColor: "rgba(34,211,238,0.08)",
          background: "rgba(34,211,238,0.02)",
        }}
      >
        <div className="w-full">
          <p className="text-center text-xs font-mono font-semibold uppercase tracking-widest text-slate-600 mb-10">
            Mayday calls we've heard before
          </p>
          <div className="grid sm:grid-cols-3 gap-5">
            {painPoints.map((item: PainPoint) => (
              <div
                key={item.headline}
                className="rounded-lg p-6"
                style={{
                  background: "rgba(180,30,20,0.07)",
                  border: "1px solid rgba(180,30,20,0.2)",
                }}
              >
                <div className="flex items-start gap-2 mb-3">
                  <span className="text-red-500 font-mono text-xs font-bold mt-0.5 flex-shrink-0">
                    MAYDAY
                  </span>
                </div>
                <p className="text-sm font-semibold text-red-200 leading-snug mb-3">
                  {item.headline}
                </p>
                <p className="text-xs text-slate-600 leading-relaxed">
                  {item.body}
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Features ── */}
      <section className="py-28 px-6 lg:px-12 xl:px-24">
        <div className="w-full">
          <div className="text-center mb-16">
            <p className="text-xs font-mono uppercase tracking-widest text-slate-600 mb-4">
              Systems check
            </p>
            <h2 className="text-3xl sm:text-4xl font-black tracking-tight mb-4">
              Four systems. One gateway.
            </h2>
            <p className="text-slate-400 text-lg max-w-xl mx-auto">
              Everything a production agent fleet needs — shipped as a single
              drop-in endpoint.
            </p>
          </div>

          <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-5">
            {features.map((f: Feature) => (
              <div
                key={f.title}
                className={`rounded-xl border p-8 transition-all duration-300 ${f.border}`}
                style={{ background: "rgba(13,21,32,0.8)" }}
              >
                <div className="flex items-center gap-3 mb-5">
                  <div className={`${f.color}`}>{f.icon}</div>
                  <div>
                    <p className="text-xs font-mono font-bold tracking-widest text-slate-600">
                      {f.tag}
                    </p>
                    <h3 className="font-bold text-white">{f.title}</h3>
                  </div>
                </div>
                <ul className="space-y-3">
                  {f.points.map((pt: string) => (
                    <li
                      key={pt}
                      className="flex items-start gap-2.5 text-sm text-slate-400"
                    >
                      <CheckIcon />
                      {pt}
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Dashboard mockup ── */}
      <section
        className="py-24 px-6 lg:px-12 xl:px-24 border-t"
        style={{ borderColor: "rgba(34,211,238,0.08)" }}
      >
        <div className="w-full">
          <div className="text-center mb-12">
            <p className="text-xs font-mono uppercase tracking-widest text-slate-600 mb-4">
              Tower view
            </p>
            <h2 className="text-3xl sm:text-4xl font-black tracking-tight mb-4">
              Full situational awareness.
              <br />
              Always.
            </h2>
            <p className="text-slate-400 text-lg max-w-xl mx-auto">
              Every agent, every call, every cost — in one place. Circuit
              breakers, guardrail fires, PII detections, and live traces,
              exactly as they happen.
            </p>
          </div>
          <DashboardMockup />
        </div>
      </section>

      {/* ── How it works ── */}
      <section
        className="py-24 px-6 lg:px-12 xl:px-24 border-t"
        style={{ borderColor: "rgba(34,211,238,0.08)" }}
      >
        <div className="w-full">
          <div className="text-center mb-16">
            <p className="text-xs font-mono uppercase tracking-widest text-slate-600 mb-4">
              Departure sequence
            </p>
            <h2 className="text-3xl sm:text-4xl font-black tracking-tight mb-4">
              Cleared for takeoff in four steps
            </h2>
            <p className="text-slate-400 text-lg">
              No new infrastructure. No agent rewrites.
            </p>
          </div>

          <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-6">
            {steps.map((s: Step) => (
              <div
                key={s.waypoint}
                className="rounded-lg p-6"
                style={{
                  background: "rgba(13,21,32,0.6)",
                  border: "1px solid rgba(34,211,238,0.08)",
                }}
              >
                <span className="block font-mono text-xs font-bold text-cyan-800 mb-4 tracking-widest">
                  {s.waypoint}
                </span>
                <h3 className="font-bold text-white mb-2">{s.title}</h3>
                <p className="text-sm text-slate-400 leading-relaxed">
                  {s.body}
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Integrations ── */}
      <section
        className="py-20 px-6 lg:px-12 xl:px-24 border-t"
        style={{
          borderColor: "rgba(34,211,238,0.08)",
          background: "rgba(34,211,238,0.015)",
        }}
      >
        <div className="w-full">
          <p className="text-xs font-mono uppercase tracking-widest text-slate-600 text-center mb-12">
            Compatible aircraft
          </p>
          <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {integrations.map((i: Integration) => (
              <div
                key={i.label}
                className="rounded-lg p-5"
                style={{
                  background: "rgba(13,21,32,0.8)",
                  border: "1px solid rgba(34,211,238,0.1)",
                }}
              >
                <p className="font-bold text-sm text-white mb-1">{i.label}</p>
                <p className="font-mono text-xs text-cyan-700 mb-2">{i.mono}</p>
                <p className="text-xs text-slate-500 leading-relaxed">
                  {i.desc}
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Crew credentials (aigw CLI / cost control) ── */}
      <section
        className="py-24 px-6 lg:px-12 xl:px-24 border-t"
        style={{
          borderColor: "rgba(34,211,238,0.08)",
          background: "rgba(34,211,238,0.015)",
        }}
      >
        <div className="w-full">
          <div className="text-center mb-14">
            <p className="text-xs font-mono uppercase tracking-widest text-slate-600 mb-4">
              Crew credentials
            </p>
            <h2 className="text-3xl sm:text-4xl font-black tracking-tight mb-4">
              Give your crew logins,
              <br />
              <span style={{ color: "#22d3ee" }}>not the master key.</span>
            </h2>
            <p className="text-slate-400 text-lg max-w-2xl mx-auto">
              Copying the company provider key into every developer, QA, and CI
              dotfile means one bill you can't break down, no per-person cap, and
              a long-lived secret on every laptop. The{" "}
              <span className="font-mono text-cyan-400">aigw</span> CLI replaces
              that key with a login.
            </p>
          </div>

          <div className="grid lg:grid-cols-2 gap-6 items-start max-w-5xl mx-auto">
            {/* Terminal */}
            <div
              className="rounded-xl overflow-hidden shadow-2xl"
              style={{
                background: "#050d14",
                border: "1px solid rgba(34,211,238,0.15)",
              }}
            >
              <div
                className="flex items-center gap-1.5 px-4 py-3 border-b"
                style={{
                  background: "rgba(34,211,238,0.04)",
                  borderColor: "rgba(34,211,238,0.1)",
                }}
              >
                <div className="w-2.5 h-2.5 rounded-full bg-red-500/60" />
                <div className="w-2.5 h-2.5 rounded-full bg-yellow-500/60" />
                <div className="w-2.5 h-2.5 rounded-full bg-green-500/60" />
                <span className="ml-3 text-xs font-mono text-cyan-900">
                  ~/dev — zsh
                </span>
              </div>
              <div className="p-6 font-mono text-sm leading-relaxed space-y-2 overflow-x-auto">
                <div className="whitespace-nowrap">
                  <span className="text-cyan-600">$</span>{" "}
                  <span className="text-slate-200">aigw login</span>
                </div>
                <div className="whitespace-nowrap text-slate-600">
                  ✓ authenticated as dev@acme.com · key minted for tenant
                  &lt;acme&gt;
                </div>
                <div className="whitespace-nowrap pt-2">
                  <span className="text-cyan-600">$</span>{" "}
                  <span className="text-slate-200">
                    aigw run openai -- python app.py
                  </span>
                </div>
                <div className="whitespace-nowrap text-slate-600">
                  ↳ OPENAI_API_KEY + OPENAI_BASE_URL set for this process only
                </div>
                <div className="whitespace-nowrap text-slate-600">
                  ↳ routed through the gateway · every call traced &amp; billed
                  to dev@acme.com
                </div>
              </div>
              <div
                className="flex items-center justify-between px-6 py-3 border-t"
                style={{
                  borderColor: "rgba(34,211,238,0.08)",
                  background: "rgba(34,211,238,0.02)",
                }}
              >
                <span className="text-xs font-mono text-cyan-900">
                  SSO login · no provider key on the machine
                </span>
                <span
                  className="text-xs font-mono px-2 py-0.5 rounded"
                  style={{
                    background: "rgba(34,211,238,0.08)",
                    color: "#22d3ee",
                  }}
                >
                  KEY ON TOWER
                </span>
              </div>
            </div>

            {/* Benefits */}
            <div className="space-y-4">
              {[
                {
                  tag: "MANIFEST",
                  title: "Every call attributed",
                  body: "Keys are tenant-, agent-, or user-scoped. Spend breaks down per team and per person — not one opaque invoice you reverse-engineer at month end.",
                },
                {
                  tag: "GROUNDED",
                  title: "Revoke one seat, not the fleet",
                  body: "Someone leaves? Revoke their access. The shared key stays valid for everyone else — no rotation scramble, no redeploy.",
                },
                {
                  tag: "BLAST RADIUS",
                  title: "No key on the tarmac",
                  body: "Provider credentials live only on the gateway. There is nothing in a developer dotfile or CI log to leak.",
                },
              ].map((b) => (
                <div
                  key={b.tag}
                  className="rounded-lg p-5"
                  style={{
                    background: "rgba(13,21,32,0.8)",
                    border: "1px solid rgba(34,211,238,0.1)",
                  }}
                >
                  <p className="text-xs font-mono font-bold tracking-widest text-cyan-800 mb-2">
                    {b.tag}
                  </p>
                  <h3 className="font-bold text-white mb-2">{b.title}</h3>
                  <p className="text-sm text-slate-400 leading-relaxed">
                    {b.body}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* ── Deployment ── */}
      <section
        className="py-24 px-6 lg:px-12 xl:px-24 border-t"
        style={{ borderColor: "rgba(34,211,238,0.08)" }}
      >
        <div className="w-full text-center">
          <p className="text-xs font-mono uppercase tracking-widest text-slate-600 mb-4">
            Deployment class
          </p>
          <h2 className="text-3xl font-black mb-4">
            Your airspace, your rules
          </h2>
          <p className="text-slate-400 mb-12">
            SaaS, dedicated, or fully self-hosted. Your data never crosses a
            region boundary without permission.
          </p>
          <div className="grid sm:grid-cols-3 gap-4">
            {deploymentTiers.map((d: DeploymentTier) => (
              <div
                key={d.tier}
                className="rounded-xl p-6 text-left"
                style={{
                  background: "rgba(13,21,32,0.8)",
                  border: "1px solid rgba(34,211,238,0.1)",
                }}
              >
                <p className="text-xs font-mono font-bold tracking-widest text-cyan-800 mb-2">
                  {d.tag}
                </p>
                <p className="font-bold text-white mb-3">{d.tier}</p>
                <p className="text-sm text-slate-500">{d.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── CTA ── */}
      <section
        id="waitlist"
        className="py-28 px-6 lg:px-12 xl:px-24 border-t"
        style={{ borderColor: "rgba(34,211,238,0.08)" }}
      >
        <div className="relative max-w-2xl mx-auto text-center">
          {/* Radar glow behind CTA */}
          <div
            className="absolute inset-0 rounded-full pointer-events-none"
            style={{
              background:
                "radial-gradient(ellipse at center, rgba(34,211,238,0.07) 0%, transparent 70%)",
            }}
          />
          <div className="relative">
            <p className="text-xs font-mono uppercase tracking-widest text-cyan-700 mb-6">
              Tower to agent fleet
            </p>
            <h2 className="text-4xl sm:text-5xl font-black tracking-tight mb-4">
              Ready to put your agents
              <br />
              <span style={{ color: "#22d3ee" }}>under control?</span>
            </h2>
            <p className="text-slate-400 text-lg mb-10 leading-relaxed">
              Join teams already using AI Gateway to get full observability,
              guardrails, and PII protection — without touching a single agent.
            </p>
            <div className="flex justify-center">
              <EmailCapture size="large" />
            </div>
            <p className="mt-5 text-sm font-mono text-slate-700">
              Prefer to talk first?{" "}
              <a
                href="mailto:bchaitanya15@gmail.com"
                style={{ color: "#22d3ee" }}
                className="underline underline-offset-4 hover:opacity-80 transition-opacity"
              >
                bchaitanya15@gmail.com
              </a>
            </p>
          </div>
        </div>
      </section>

      {/* ── Footer ── */}
      <footer
        className="border-t py-10 px-6 lg:px-12 xl:px-24"
        style={{ borderColor: "rgba(34,211,238,0.08)" }}
      >
        <div className="w-full flex flex-col sm:flex-row items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <svg viewBox="0 0 32 32" fill="none" className="w-4 h-4 opacity-40">
              <circle
                cx="16"
                cy="16"
                r="14"
                stroke="#22d3ee"
                strokeWidth="2"
                strokeOpacity="0.2"
              />
              <circle
                cx="16"
                cy="16"
                r="8"
                stroke="#22d3ee"
                strokeWidth="2"
                strokeOpacity="0.4"
              />
              <circle cx="16" cy="16" r="3" fill="#22d3ee" />
              <path
                d="M16 4V16L24 24"
                stroke="#22d3ee"
                strokeWidth="2.5"
                strokeLinecap="round"
              />
            </svg>
            <span className="font-mono font-black text-xs tracking-widest text-slate-700 uppercase">
              AI <span className="text-slate-600">Gateway</span>
            </span>
          </div>
          <p className="text-xs font-mono text-slate-700">
            © {new Date().getFullYear()} AI Gateway — Built for the agent era.
          </p>
          <a
            href="mailto:bchaitanya15@gmail.com"
            className="text-xs font-mono text-slate-700 hover:text-slate-500 transition-colors"
          >
            bchaitanya15@gmail.com
          </a>
        </div>
      </footer>
    </div>
  );
}
