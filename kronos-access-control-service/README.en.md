# kronos-access-control-service

[简体中文](./README.md) | **English**

The gateway's authorization service: it implements `AuthService.Authorities` from the contract hub's `access_control` module.

> ⚠️ **`service/auth` is a stub**: no signature check, no revocation, no expiry, so any string is a valid token. It **refuses to start** at production level. Where to plug in a real implementation is below.

## Where it sits

The gateway is the only process that speaks HTTP, and it asks this service before forwarding:

```
Browser ──HTTP──▶ Gateway ──Authorities(path, method)──▶ this service
                    │                                      │
                    │◀──────── allowed + Payload ──────────┘
                    │
                    └──gRPC (payload in headers)──▶ business service / key-service
```

The four templates:

| Template | Role | gRPC | Own HTTP |
|---|---|---|---|
| `kronos-service-template` | Business service with a database | yes | yes |
| `kronos-key-service-template` | Infrastructure: keys and ids | yes | no |
| **this one** | Authorization | yes | no, deliberately |
| `kronos-gateway-template` | The HTTP face | no | yes |

## Two constraints that matter

### Authorities deliberately has no HTTP route

The contract carries **no** `google.api.http` annotation for it. A route would let anyone who can reach the gateway call authorization directly, and its response carries the caller's `Payload` — a public token-for-identity endpoint that bypasses every business check.

This service therefore needs no HTTP gateway rebuild at all; it has no HTTP face.

### Downstream services do not re-verify the token

Verifying once is enough. Verifying twice means two implementations that must agree forever and eventually will not, where "gateway allows, downstream rejects" is the good outcome and the reverse is an incident.

**That constraint requires downstream services never to be exposed directly.** They trust the `Payload` in the header, so anything that can reach them can forge any user. It is enforced by network policy at deployment time rather than by code — which must be stated, or "downstream does not verify" reads as an oversight instead of a design.

## Replacing the stub

`service/auth/auth_service.go` carries three `TODO`s, in order:

| # | What to do | What goes wrong |
|---|---|---|
| 1 | Parse and verify the token: signature, expiry, revocation | A failure must return `TokenValid=false`, a 401. Returning `Allowed=false` with `TokenValid=true` tells the frontend it lacks permission and keeps it from the login page |
| 2 | Load the user and roles into the `Payload` | Omitting `tenant_id` voids **every** downstream tenant check: downstream receives an empty string, and an empty tenant usually means no filtering at all |
| 3 | Ask a policy engine about `(user, path, method)` | On a denial `TokenValid` stays `true`, so the gateway answers 403 rather than 401 |

`AuthoritiesPolicy`'s `v1` and `v2` exist to feed a Casbin enforcer directly, so step 3 is usually one `enforcer.Enforce(roleCode, v1, v2)` call.

Afterwards:

1. Delete the production-level refusal in `bootstrap/server.go`
2. Add the `cache` section back to `resources/` — `Authorities` is the **hottest RPC** on the path, called before every gateway request, so the revocation list, users, roles and policies must all come from cache rather than a database per call

## The allow list

`publicPaths` in `constants/service_name.go` holds only paths that genuinely need no credentials: health checks, the docs site and metrics.

**Business paths do not belong there.** The allow list is a bypass, and each entry is one more entrance that authorization never covers; because it works, nobody revisits whether it should be there. An anonymous business endpoint belongs in the policy engine as an anonymous role.

Prefix matching has a known side effect: `/health` also matches `/healthcheck-admin`. The test asserts that too — not because it is right, but because it is the current behaviour, so the test will prompt you to fix it when the matching is reworked.

`IsPublicPath` skips empty prefixes explicitly. That is the allow list's most dangerous failure mode: one empty string makes `strings.HasPrefix` true for every path, leaving the whole service credential-free while every endpoint works and nothing is logged. `TestIsPublicPathRejectsEmptyPrefix` keeps that line in place.

## Quick start

```bash
kopy new https://github.com/kamalyes/kronos-access-control-template -o ./kronos-access-control-service
cd kronos-access-control-service
go run .
```

The stub has no external dependencies: no database and no Redis.

```bash
# Verify reflection, which the gateway uses to discover this service
grpcurl -plaintext localhost:9390 list

# An allow-listed path passes with no token
grpcurl -plaintext \
  -d '{"policy":{"v1":"/health","v2":"GET"}}' \
  localhost:9390 kronos.api.access_control.AuthService/Authorities

# A business path with no token: allowed=false, token_valid=false, so the gateway answers 401
grpcurl -plaintext \
  -d '{"policy":{"v1":"/v1/products","v2":"GET"}}' \
  localhost:9390 kronos.api.access_control.AuthService/Authorities

# With any token at all the stub allows it and returns a fixed identity
grpcurl -plaintext \
  -H 'authorization: Bearer whatever' \
  -d '{"policy":{"v1":"/v1/products","v2":"GET"}}' \
  localhost:9390 kronos.api.access_control.AuthService/Authorities
```

## A known trade-off

`service/auth` embeds `pb.UnimplementedAuthServiceServer`, so a new RPC in the contract does **not** break the build and instead returns `codes.Unimplemented` at runtime — which, from the gateway, looks like the authorization service being down.

Dropping the embed would catch it at compile time but make every new RPC in the contract break this template's build immediately, which is too brittle for a stub. The latter was chosen and the cost is recorded here.

## The service name must match the gateway's config

`constants.ServiceName` is three things at once: the config file prefix, this service's registered name, and **the key under the gateway's `grpc.clients`**. The gateway looks the connection up by service name to call `Authorities`, and a mismatch makes every gateway request report `auth service connection is nil` while this service is plainly alive.
