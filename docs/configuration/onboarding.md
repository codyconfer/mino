# Onboarding

Onboarding requires GitHub authenticated **and** a GPG (or SSH) signing key that git
uses and GitHub has verified. Mino classifies you as **unauthenticated** (no GitHub
auth at all), **unauthorized** (authed but a signing/scope/verification gap), or
**authorized**, and gates each mode differently:

| Mode | unauthenticated | unauthorized | authorized |
|---|---|---|---|
| **cli** | prompt to authenticate, then guided setup; errors block | warn + continue by default; **block** in an `ALL_OR_NOTHING_AUTH` build | run |
| **serve** | warn in logs, run anyway | warn in logs, run anyway | run |
| **daemon** | warn in logs | warn in logs | run |
| **deck** | run the guided onboarding flow, then continue | run the guided flow, then continue | run |

```sh
mino onboard            # guided check + fix instructions, loops until ready
mino onboard --status   # print the checklist without changing anything
mino onboard --reset    # clear the marker (re-onboard on the next run)
```

`onboard` checks four things and, for any gap, prints the exact commands to fix it
(it never generates keys or edits your git config): (1) GitHub auth — `gh` CLI or a
cached token; (2) `git config user.signingkey` is set; (3) that secret key is in
your local GPG keyring; (4) the key's public half is registered on your GitHub
account, so signed commits show **Verified**. `mino verify onboarding` reports the
same checklist. `login`, `verify`, `install`/`clean`/`nuke`, and `--help` skip the
gate entirely. The onboarded state lives in `settings.yaml`.

**Domain-locked builds.** A distribution can be compiled to onboard *only* when the
signing key has a GitHub-verified identity in a specific email domain:

```sh
make package EMAIL_DOMAIN=example.com   # only unlocks for @example.com identities
```

This adds a fifth onboarding check (a verified `@example.com` email on the
registered key) and stamps the domain into the marker, so a binary built for one
domain won't accept an onboarding done by an unrestricted build. Built without the
flag, Mino has no domain restriction. Note this is a distribution-policy control,
not a hardened security boundary — `settings.yaml` is user-writable.

## All-or-nothing auth (`ALL_OR_NOTHING_AUTH`)

`ALL_OR_NOTHING_AUTH` is a **build-time policy**, not a runtime environment
variable or config setting. Set it while compiling to make ordinary cli
directives return an error instead of continuing when the user is authenticated
but not fully authorized:

```sh
make command ARGS="fly work" ALL_OR_NOTHING_AUTH=1   # cli requires full authorization
make package ALL_OR_NOTHING_AUTH=1                    # build all-or-nothing releases
```

The value is enabled when non-empty; use `1` by convention and omit the variable
to build the default warning-only behavior.

This switch is deliberately narrow:

- It changes only the **cli + unauthorized** case.
- It does not change `serve`, `daemon`, or `deck` behavior.
- It does not change unauthenticated cli behavior: Mino still launches guided
  authentication, and an authentication/onboarding error already blocks.
- It does not block gate-exempt recovery commands such as `login`, `verify`,
  `install`, `clean`, `nuke`, or `--help`.
- In a domain-locked build, failing the domain check counts as unauthorized, but
  blocks cli directives only when `ALL_OR_NOTHING_AUTH` was also enabled.
