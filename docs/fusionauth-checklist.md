# FusionAuth checklist for the Go rewrite

Three things about the current FusionAuth application have to be confirmed —
and probably changed — before login works on the new site. All of them need
admin access to <https://auth.itemize.no/admin>; none of them can be checked
from the code.

Do these on a **staging application first** if you have one. Changing the grant
type or the signing key affects the live site immediately.

FusionAuth moves its admin labels around between versions, so treat the
navigation below as "look for something like this" rather than exact clicks.

---

## 1. Enable the Authorization Code grant

**Why.** The old server used `express-openid-connect`, whose v2 default is
`response_type=id_token` with `response_mode=form_post` — the implicit flow. The
browser gets the token straight from the authorization endpoint and POSTs it
back. The Go rewrite uses the authorization-code flow with PKCE instead: the
browser only ever receives a short-lived code, and the token is fetched
server-to-server. That is the modern default and the implicit flow is
discouraged precisely because tokens end up in the browser.

**Where.** Applications → *Itemize* → ✎ Edit → **OAuth** tab.

**Check:**

- **Enabled grants** — `Authorization Code` must be ticked. `Implicit` probably
  is today; leave it on until the cutover, then remove it.
- **Authorized redirect URLs** — must contain exactly `https://itemize.no/callback`,
  with no trailing slash. Add `http://localhost:3000/callback` (and `:3123` if
  you use the port I've been testing on) for local development.
- **Require PKCE** — safe to enable. The rewrite always sends a PKCE challenge.
- **Logout URL** — should be `https://itemize.no`. The old config had
  `idpLogout: false`, so logging out clears our session without ending the
  FusionAuth one; the rewrite keeps that behaviour.

**What breaks if you skip it.** Login fails at the callback with an
`unsupported_grant_type` or `invalid_grant` error from the token endpoint.

---

## 2. Move the ID token from HS256 to RS256

**Why.** The old config pinned `idTokenSigningAlg: 'HS256'`, meaning the ID
token is HMAC-signed with a shared secret. Two problems:

1. Anyone holding that secret can **mint** tokens, not just verify them. With
   RS256 only FusionAuth holds the private key and we verify with the public
   one. For an information-security organisation this is the easy call.
2. The `go-oidc` library verifies against the provider's published JWKS, which
   by definition contains no symmetric keys. Supporting HS256 means hand-writing
   the signature check — doable, but it is security-critical code we would then
   own, and the whole point of the rewrite is to own less.

**Where.** Applications → *Itemize* → ✎ Edit → **JWT** tab.

**Check:**

- Is **Enabled** (the per-application JWT override) on?
  - **On** → the keys are set here. Look at **Id token signing key**.
  - **Off** → it inherits from Tenants → *your tenant* → ✎ Edit → **JWT** tab.
    Check there instead.
- If the key is an **HMAC** key, that is HS256 — this is the thing to change.

**To change it:**

1. Settings → **Key Master** → *Generate RSA key pair* (2048 bits is fine). Give
   it a name you will recognise, e.g. `itemize-id-token`.
2. Back in the JWT tab, set **Id token signing key** to that key. Set **Access
   token signing key** to it as well while you are there.
3. Save.

**Verify:** load `https://auth.itemize.no/.well-known/openid-configuration` and
confirm `id_token_signing_alg_values_supported` lists `RS256`. Then confirm
`https://auth.itemize.no/.well-known/jwks.json` returns a key — with HS256 only,
it comes back empty.

**If RS256 is genuinely not an option**, tell me and I will implement the HMAC
verifier instead. In that case I also need to know *which* secret signs the
token: the OIDC spec says the client secret, but FusionAuth's own default is a
separately generated HMAC key. Those are different values and the wrong one
fails every login.

---

## 3. Confirm the ID token actually carries `roles`, `fullName` and `imageUrl`

**Why this one matters most.** The old server read these three fields straight
off the ID token:

```js
const { name, roles, email, sub, imageUrl, fullName } = { roles: [], ...req.oidc?.user };
```

`sub` and `email` are standard OIDC claims. `roles` FusionAuth adds for the
application. But **`fullName` and `imageUrl` are not OIDC claims at all** —
those are FusionAuth's internal field names, which strongly suggests an **ID
token populate lambda** is adding them.

If that lambda is missing or is not wired to the application, the failure is
silent and asymmetric:

- `fullName` missing → the check-in attendance list records blank names, because
  `check_in.attendances[].name` is written from it.
- `imageUrl` missing → the profile page falls back to the logo. Harmless.
- **`roles` missing → nobody has `Styret`.** Event administration and check-in
  simply vanish for everyone, and it looks like a permissions bug rather than a
  configuration one. This is the one that will waste your afternoon.

**Where.** Applications → *Itemize* → ✎ Edit → **JWT** tab → look for **Id token
populate lambda**. If one is set, read it under Settings → **Lambdas**. It
should be assigning `jwt.roles`, `jwt.fullName` and `jwt.imageUrl`.

**How to actually see the claims.** The reliable way is to complete a real login
and decode the token. Using the Login API against a test account:

```sh
# Application Id is on Applications → Itemize (the UUID in the list)
curl -s -X POST https://auth.itemize.no/api/login \
  -H "Authorization: $FUSION_AUTH_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"loginId":"you@example.com","password":"…","applicationId":"<APPLICATION_ID>"}' \
| python3 -c 'import sys,json,base64;t=json.load(sys.stdin)["token"];p=t.split(".")[1];print(json.dumps(json.loads(base64.urlsafe_b64decode(p+"="*(-len(p)%4))),indent=2,ensure_ascii=False))'
```

One caveat: this returns the **access token**, not the ID token. They are signed
the same way and a populate lambda usually fills both, but they are configured
separately — so if `roles` shows up here, also confirm the *Id token* populate
lambda is set, not only the access token one.

Once Phase 4 lands I will log the full claim key set once at debug level on
startup, which makes this a one-line check rather than a curl recipe. But it is
worth knowing before then, because it determines whether a lambda needs writing.

---

## What I need back

1. Authorization Code grant enabled, and `https://itemize.no/callback` registered? ✅ / ❌
2. ID token signing — RS256 now, or must it stay HS256? If it must stay, which secret signs it?
3. The claim list from the token above — specifically whether `roles`, `fullName` and `imageUrl` are present.

With those three answers Phase 4 is unblocked. Everything up to and including
Phase 3 works without them.

---

## Unrelated but adjacent

`FUSION_AUTH_API_TOKEN` is optional in the new config. Left empty, login still
works but registration and profile writes return 503. That is deliberate, so a
contributor can run the site locally without holding a production API key.
