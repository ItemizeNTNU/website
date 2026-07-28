# Authentication

Login runs against FusionAuth over OpenID Connect, using the authorization-code
flow with PKCE. The session is a sealed cookie; there is no session store.

- Code: [`internal/auth/`](../internal/auth/)
- FusionAuth admin steps: [`fusionauth-checklist.md`](fusionauth-checklist.md)

---

## The open item: HS256 → RS256

**Today the site verifies ID tokens with HS256, and that is a compromise, not a
preference.**

Under HS256 the token is signed with a secret that both parties hold. Anyone
with that secret can **mint** an ID token, not merely check one — so the secret
is a forgery key, and it lives in the environment of every service that
validates a login. Under RS256, FusionAuth holds a private key and signs; we
hold only the public key and verify. We could not forge a token if we wanted to,
and neither could anything that reads our environment.

It is also the reason there is hand-written cryptography in this repository at
all. `go-oidc` verifies against the provider's published JWKS, which contains
only public keys. HS256 is symmetric, so there is nothing in the JWKS to verify
against and the library has no path for it. That is what
[`hs256.go`](../internal/auth/hs256.go) exists to fill.

### Why it has not been done yet

The FusionAuth tenant is shared with other applications, including the wiki, and
it was not clear whether the ID-token signing key is configured per application
or for the whole tenant. Changing a tenant-wide key would affect every
application at once.

**The thing to check:** Applications → *Itemize* → ✎ Edit → **JWT** tab, and
whether the per-application **Enabled** override is on.

- **Override on** → the keys are set here, and switching Itemize to RS256 is
  invisible to the wiki and everything else on the tenant. This is the safe
  path.
- **Override off** → Itemize inherits from Tenants → ✎ Edit → **JWT**. Turn the
  per-application override *on* first, copy the current settings into it, and
  only then change the algorithm. That converts a shared change into a scoped
  one.

### Making the change

1. Settings → **Key Master** → *Generate RSA key pair* (2048 bits). Name it
   something recognisable, e.g. `itemize-id-token`.
2. Applications → *Itemize* → **JWT** tab → set **Id token signing key** (and
   **Access token signing key**) to it.
3. Verify: `https://auth.itemize.no/.well-known/jwks.json` returns a key. Under
   HS256 it comes back empty.
4. Deploy with `FUSION_AUTH_ID_TOKEN_ALG=RS256`.
5. Log in. If it works, the migration is done.

### Cleaning up afterwards

Once RS256 has been running for a release or two:

- Delete [`internal/auth/hs256.go`](../internal/auth/hs256.go) and its tests.
- Remove the `HS256` branch in `auth.New`.
- Change the default of `FUSION_AUTH_ID_TOKEN_ALG` in
  [`internal/config/config.go`](../internal/config/config.go) to `RS256`, or
  drop the setting entirely.
- Drop `FUSION_AUTH_ID_TOKEN_HMAC_SECRET` from `.env.example`.

Each of those places carries a `TODO(auth)` comment pointing back here.

### If HS256 has to stay

It is not catastrophic — the token arrives from the token endpoint over TLS,
never via the browser, so an attacker would need the secret to exploit the
weakness. But it is worth knowing which secret that is.

`FUSION_AUTH_ID_TOKEN_HMAC_SECRET` exists for this. The OIDC specification says
the HMAC key for `HS*` ID tokens is the client secret, and that is the default
here. **FusionAuth's own default is a separately generated HMAC key**, which is
a different value. If logins fail with a signature error, that is why: find the
key under the JWT tab, and set the variable.

---

## The flow

```
GET /login
  ├─ generate state, nonce and a PKCE verifier
  ├─ seal all three, plus return_to, into a short-lived cookie
  └─ 302 to FusionAuth

GET /callback
  ├─ open the flow cookie and expire it (one attempt per cookie)
  ├─ constant-time compare the state          ← CSRF defence for the callback
  ├─ exchange the code, sending the PKCE verifier
  ├─ verify the ID token signature and the nonce
  ├─ build the session and seal it into a cookie
  └─ 302 to return_to

GET /logout
  └─ clear the session cookie and 302 home
```

**PKCE is always sent**, whether or not the FusionAuth application requires it.
A provider validates the verifier whenever a challenge was present, so we get
the protection regardless of that setting — which means "Require PKCE" can stay
off without weakening this site.

**Logout is local.** The previous site ran with `idpLogout` disabled, so signing
out here has never signed you out of FusionAuth. That behaviour is preserved; a
silent change would surprise people.

---

## The session cookie

`itemize_session`, AES-256-GCM sealed, `HttpOnly`, `SameSite=Lax`, `Secure`
whenever `BASE_URL` is https. The key is SHA-256 of `FUSION_AUTH_SECRET`, which
config refuses to accept below 32 bytes.

It carries the user's id, name, email, avatar URL and roles — and deliberately
**not** the access or ID token. Nothing needs them after login (server-to-server
calls use the API key), so leaving them out means a leaked cookie yields a name
rather than a working credential.

`SameSite=Lax` rather than `Strict` is required, not a preference: `Strict`
would drop the cookie on the top-level redirect back from FusionAuth, so a
visitor would arrive logged in and immediately appear logged out.

Lifetime is the ID token's expiry, capped at seven days.

### At cutover

The cookie is named differently from the Sapper site's `appSession`, so every
signed-in member is logged out once when this deploys. That is expected. The old
cookie is explicitly expired rather than left in the jar.

---

## Roles

Authorisation is one role, `Styret`, read from the ID token's `roles` claim.

`roles`, `fullName` and `imageUrl` are **not** standard OIDC claims — they come
from a FusionAuth ID-token populate lambda. If that lambda is missing or is not
wired to the application, `roles` arrives empty, **nobody has `Styret`**, and
event administration and check-in silently vanish for everyone. It presents as a
permissions bug rather than a configuration one, which is why `auth.New` logs a
line at debug level when a token carries no roles.

`parseRoles` accepts both a JSON list and a bare string, because FusionAuth
emits a single role as a plain string under some lambda configurations.

---

## Environment

| Variable | Default | Notes |
|---|---|---|
| `FUSION_AUTH_HOST` | — | Must match the `issuer` in the discovery document exactly, or startup fails |
| `FUSION_AUTH_CLIENT_ID` | — | |
| `FUSION_AUTH_CLIENT_SECRET` | — | Also the default HS256 verification key |
| `FUSION_AUTH_SECRET` | — | Session encryption key, ≥ 32 bytes |
| `FUSION_AUTH_API_TOKEN` | *(empty)* | Optional; without it registration and profile writes return 503 but login works |
| `FUSION_AUTH_ID_TOKEN_ALG` | `HS256` | `HS256` or `RS256` — see above |
| `FUSION_AUTH_ID_TOKEN_HMAC_SECRET` | *(client secret)* | Only if FusionAuth signs with a separate HMAC key |

---

## Tests

[`internal/auth/auth_test.go`](../internal/auth/auth_test.go) covers the parts
that fail silently or dangerously:

- session round-trip, wrong key, tampering, garbage, expiry, lifetime cap
- HS256 verification, including **algorithm confusion** — `alg: none` and an
  RS256 token re-signed as HS256 must both be refused, because a verifier that
  dispatches on the token's own header can be talked into accepting anything
- the roles claim in every shape FusionAuth emits
- the full gating matrix: {anonymous, member, Styret} × {login-only, role-gated}
  × {page, API}
- `return_to` as an open redirect — `//evil.example` and friends
