# Cloudflare proxy for food.ianmyjer.com

Notes on whether/how to put this app behind Cloudflare's proxy (orange
cloud) vs. DNS-only (gray cloud). Current deployment: Cloud Run, custom
domain `food.ianmyjer.com`, DNS via Cloudflare.

## What proxying buys you

Ranked by relevance to this app.

### Likely worth it

- **Hides the `*.run.app` URL.** Cloud Run services are directly reachable
  at their run.app URL, which bypasses anything in front of them —
  including the per-IP rate limiter in `internal/ratelimit`. With
  proxying on *and* Cloud Run ingress locked to Cloudflare's IP ranges,
  you get a real chokepoint.
- **DDoS absorption.** Cloud Run autoscales on request count, so a flood
  of junk traffic is a flood of billing. Cloudflare absorbs L3/L4
  attacks for free and can rate-limit at the edge, so bad traffic never
  spins up Cloud Run instances. This matters more than usual here
  because the backend calls a paid LLM (Gemini) — a modest attack could
  run up real API costs before the in-process limiter kicks in.
- **Edge caching of static assets.** The Svelte build output under
  `/assets/*` is fingerprinted and immutable. Cloudflare can serve it
  from its edge cache for free, taking load off Cloud Run and improving
  first paint globally (especially on cold starts).
- **Free analytics.** Cloudflare shows traffic / country / top paths
  without needing a client-side tracker.

### Nice but marginal

- **WAF rules** (SQLi, common exploits). This app has almost no attack
  surface — no SQL, no uploads, typed JSON APIs only — so the WAF mostly
  catches noise.
- **Bot Fight Mode / challenges** in front of `/auth/login` as a second
  layer above the per-IP limiter.
- **Page / Transform rules.** Cloud Run already handles HTTPS, so most
  of these would be redundant.
- **Free TLS cert.** Cloud Run's managed cert already covers the custom
  domain.

### Not relevant

- CDN for user uploads — there are none.
- Workers / R2 / Images / Stream — not used.

## Costs of proxying

- **One extra hop** (~10–30 ms depending on geography). Invisible for
  SPA use in practice.
- **Cloudflare terminates TLS** and re-encrypts to Cloud Run. They see
  traffic in plaintext. Not a concern for a food tracker; worth noting.
- **Config ceremony** (see below).

## Recommendation

Turn proxying on. The two things that actually pay for themselves:

1. Closing the `*.run.app` bypass so the in-process rate limiter is
   meaningful.
2. Absorbing abuse traffic before it reaches Gemini and turns into real
   money.

Everything else is gravy. DNS-only remains acceptable for low-traffic
personal use — the per-user limiter still protects authenticated
endpoints — but the `/auth/*` abuse vector and Gemini cost risk both get
worse the moment the URL gets shared.

## Setup checklist (when enabling proxy)

1. **Cloudflare → DNS.** Flip the `food` record to proxied (orange
   cloud).
2. **Cloud Run ingress.** Lock the service to Cloudflare traffic only.
   Two options:
   - *Simpler:* HTTPS load balancer in front of Cloud Run with a Cloud
     Armor policy allowlisting Cloudflare's IP ranges
     (<https://www.cloudflare.com/ips/>). Set Cloud Run ingress to
     "Internal and Cloud Load Balancing."
   - *Cheaper / lazier:* leave ingress open and accept the spoofing
     risk. The limiter still works against honest clients; a determined
     attacker can bypass it by hitting `*.run.app` directly and setting
     `CF-Connecting-IP` to anything.
3. **Cloudflare → SSL/TLS.** Set mode to **Full (strict)**. "Flexible"
   would be insecure; "Full" doesn't verify. Cloud Run's managed cert is
   real and Full (strict) works.
4. **Cloudflare → Caching → Cache Rules.** Cache `/assets/*` aggressively
   (1 year) since the filenames are fingerprinted. Do *not* cache
   `/api/*` or `/auth/*`.
5. **Optional:** Security → Bot Fight Mode on, plus an edge rate-limiting
   rule targeting `/auth/*` as a second layer.

## App-side assumptions already in place

- `internal/ratelimit.IPKey` prefers `CF-Connecting-IP`, falls back to
  the leftmost `X-Forwarded-For`, then `r.RemoteAddr`. Works whether
  Cloudflare is proxying or DNS-only.
- `CF-Connecting-IP` is only meaningful when proxied. In DNS-only mode
  the header simply isn't present and the code falls through to
  `X-Forwarded-For` from Cloud Run's front end, which is also correct.
- `http.CrossOriginProtection` runs in prod with no trusted-origin list
  because frontend and API share an origin (Go binary embeds the SPA).
  Cloudflare proxying does not change the origin the browser sees, so
  no COP changes are needed either way.
