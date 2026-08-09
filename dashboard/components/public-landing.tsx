"use client";

import type { CSSProperties } from "react";
import { useEffect, useMemo, useState } from "react";

import { ControlMonolith, OrbitalRig, RouteIcon, SignalStamp, WireTunnel } from "./system-icons";

const showcaseCards = [
  {
    eyebrow: "Money Graph",
    titleA: "Payment",
    titleB: "Operations",
    tone: "dark",
    tags: ["Orders", "Captures", "Refunds", "Disputes"],
    body:
      "Trace checkout, authorization, refund pressure, settlement movement, and dispute risk through one commercial operating picture.",
    visual: <OrbitalRig className="showcase-visual-svg" />,
  },
  {
    eyebrow: "Runtime Control",
    titleA: "System",
    titleB: "Posture",
    tone: "light",
    tags: ["Gateway", "Sagas", "Webhooks", "Observability"],
    body:
      "Rehearse provider failure, inspect event drift, and keep asynchronous delivery health legible without moving between disconnected tools.",
    visual: <ControlMonolith className="showcase-visual-svg" />,
  },
  {
    eyebrow: "Treasury Fabric",
    titleA: "Payout",
    titleB: "Readiness",
    tone: "dark",
    tags: ["Beneficiaries", "Settlements", "Risk", "Compliance"],
    body:
      "Move from compliance and reserve posture into treasury approval, outbound transfer readiness, and merchant commercial confidence.",
    visual: <OrbitalRig className="showcase-visual-svg visual-secondary" />,
  },
] as const;

const valueCards = [
  {
    icon: "overview",
    label: "Commercial + treasury picture",
    title: "From intake to payout without context loss",
    body:
      "Keep orders, settlements, webhook delivery, payout approval, and dispute pressure connected in one system language instead of fragmented internal tooling.",
    tone: "dark",
  },
  {
    icon: "gateway",
    label: "Failure rehearsal built in",
    title: "Stress adverse paths before they happen live",
    body:
      "Drive gateway scenarios, review runtime posture, and make operator resilience visible instead of implicit.",
    tone: "light",
  },
  {
    icon: "reports",
    label: "Finance confidence",
    title: "Statements, exports, controls, and auditability",
    body:
      "Use the same product surface for reporting, tax context, risk outcomes, and audit trails that explain why state changed.",
    tone: "light",
  },
] as const;

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function CapabilityCard({
  eyebrow,
  titleA,
  titleB,
  tone,
  tags,
  body,
  visual,
  index,
}: {
  eyebrow: string;
  titleA: string;
  titleB: string;
  tone: "dark" | "light";
  tags: readonly string[];
  body: string;
  visual: React.ReactNode;
  index: number;
}) {
  return (
    <article
      className={`showcase-panel showcase-panel-${tone}`}
      style={{ top: `${88 + index * 18}px` }}
    >
      <div className="showcase-panel-backdrop">
        <WireTunnel className="showcase-wireframe" size={560} />
      </div>
      <div className="showcase-copy">
        <div className="eyebrow">{eyebrow}</div>
        <h2>
          {titleA}
          <span>{titleB}</span>
        </h2>
        <div className="showcase-tags">
          {tags.map((tag) => (
            <span className="showcase-tag" key={tag}>
              {tag}
            </span>
          ))}
        </div>
        <p>{body}</p>
      </div>
      <div className="showcase-visual-shell">
        <div className="showcase-visual">{visual}</div>
      </div>
    </article>
  );
}

export default function PublicLanding({
  action,
  redirectTo,
}: {
  action: string;
  redirectTo: string;
}) {
  const [scrollY, setScrollY] = useState(0);

  useEffect(() => {
    let frame = 0;
    const onScroll = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        setScrollY(window.scrollY || 0);
      });
    };
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", onScroll);
    };
  }, []);

  const motion = useMemo(() => {
    const heroTravel = clamp(scrollY * 0.22, 0, 180);
    const wordTravel = clamp(scrollY * 0.12, 0, 120);
    const glowTravel = clamp(scrollY * 0.08, 0, 80);
    return {
      "--hero-shift": `${heroTravel}px`,
      "--word-shift": `${wordTravel}px`,
      "--glow-shift": `${glowTravel}px`,
    } as CSSProperties;
  }, [scrollY]);

  return (
    <section className="studio-landing fade-up" style={motion}>
      <div className="scroll-hint">Scroll</div>

      <section className="hero-stage">
        <div className="hero-stage-shell">
          <div className="hero-backdrop-word">paygate</div>
          <div className="hero-grid">
            <div className="hero-stage-copy">
              <div className="eyebrow">Merchant operating system</div>
              <h1>
                Move money
                <br />
                with a premium
                <br />
                operating layer.
              </h1>
              <p className="lede">
                PayGate turns payment intake, treasury control, risk review, dispute pressure,
                webhooks, and settlement readiness into one branded merchant product instead of a
                stitched-together operator console.
              </p>
              <div className="hero-cta-row">
                <button className="primary-button" form="dashboard-login-form" type="submit">
                  Enter PayGate
                </button>
                <a className="ghost-button" href="#operator-lanes">
                  Explore operator lanes
                </a>
              </div>
              <div className="hero-ticker">
                <span>Orders</span>
                <span>Gateway control</span>
                <span>Webhooks</span>
                <span>Risk</span>
                <span>Payouts</span>
                <span>Compliance</span>
              </div>
            </div>

            <div className="hero-stage-visual">
              <div className="hero-chip">
                <SignalStamp size={84} />
              </div>
              <div className="hero-gridlines">
                <WireTunnel className="hero-gridlines-svg" size={520} />
              </div>
              <div className="hero-render">
                <OrbitalRig className="hero-render-svg" />
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="trust-ribbon">
        <div className="trust-cell">
          <span>Surface</span>
          <strong>Commercial intake, treasury, risk, webhooks, payouts</strong>
        </div>
        <div className="trust-cell">
          <span>Experience</span>
          <strong>Product-grade visuals instead of generic back-office chrome</strong>
        </div>
        <div className="trust-cell">
          <span>Signal</span>
          <strong>System posture, merchant state, and finance pressure in one frame</strong>
        </div>
      </section>

      <section className="marquee-band" id="operator-lanes">
        <div className="marquee-copy">
          <div className="eyebrow">Designed like a flagship product, not a vendor dashboard</div>
          <h2>
            Oversized typography, premium motion, and a visual system built to make money
            infrastructure feel trusted.
          </h2>
        </div>
      </section>

      <section className="showcase-stack">
        {showcaseCards.map((card, index) => (
          <CapabilityCard key={card.eyebrow} index={index} {...card} />
        ))}
      </section>

      <section className="value-grid">
        {valueCards.map((card) => (
          <article
            className={`value-card${card.tone === "dark" ? " value-card-dark" : ""}`}
            key={card.title}
          >
            <div className="page-glyph-inline">
              <RouteIcon name={card.icon} size={24} />
              <span>{card.label}</span>
            </div>
            <h3>{card.title}</h3>
            <p>{card.body}</p>
          </article>
        ))}
      </section>

      <section className="access-stage" id="operator-sign-in">
        <div className="access-panel">
          <div className="eyebrow">Operator sign-in</div>
          <h2>Authenticate into the merchant operating picture</h2>
          <p className="access-copy">
            Use the dashboard entry point to open live merchant state, commercial control, and
            treasury review surfaces from the same product frame.
          </p>
          <form action={action} className="stack" id="dashboard-login-form" method="POST">
            <input name="redirect_to" type="hidden" value={redirectTo} />
            <label>
              Merchant ID
              <input name="merchant_id" placeholder="merch_xxx" required type="text" />
            </label>
            <label>
              User Email
              <input name="email" placeholder="owner@example.com" required type="email" />
            </label>
            <label>
              Password
              <input name="password" placeholder="********" required type="password" />
            </label>
            <div className="hero-cta-row">
              <button className="primary-button" type="submit">
                Open dashboard
              </button>
              <a className="ghost-button" href="/overview">
                Preview structure
              </a>
            </div>
          </form>
        </div>

        <div className="access-aside">
          <div className="access-visual-card">
            <ControlMonolith className="aside-render-svg" />
          </div>
          <div className="access-note">
            <div className="eyebrow">Bootstrap</div>
            <p>
              Initial dashboard users can be created through the merchant bootstrap endpoint during
              setup, then managed from the team surface.
            </p>
            <code>POST /v1/merchants/{"{merchant_id}"}/users/bootstrap</code>
          </div>
        </div>
      </section>
    </section>
  );
}
