# Policy loaders

Public types live in this package. Application authors implement [PolicyLoader]
and optionally [SharedPolicySource]; the filesystem adapter is
[FilePolicyLoader]. User-facing contract: [docs/users/adapters.md](../docs/users/adapters.md).

## Plan modules

`PolicyLoader.Load` returns modules for one `(planId, version)`:

| Field | OPA package | Query |
| --- | --- | --- |
| `PolicyBundle.Inbound` | `plan.inbound` | `data.plan.inbound` |
| `PolicyBundle.Outbound` | `plan.outbound` | `data.plan.outbound` |
| `PolicyBundle.Data` | document data | `data.*` |
| `PolicyBundle.Revision` | opaque etag / hash | cache identity |

Nil bundle means no **plan** policy for that key. Tenant, DB handles, and
catalog URLs belong on the loader value, not on `PolicyRequest`.

## Org-wide modules

If the same loader also implements `SharedPolicySource`, the engine calls
`LoadShared` independently of `Load` and interns the result:

| Field | OPA package | Role |
| --- | --- | --- |
| `SharedPolicy.Inbound` | `shared.inbound` | AND with plan inbound `allow` |
| `SharedPolicy.Outbound` | `shared.outbound` | AND with plan outbound `allow` |
| `SharedPolicy.Libraries` | typically `lib.*` | compiled into shared + each plan |
| `SharedPolicy.Revision` | opaque etag / hash | cache identity |

Shared decisions must not set `hints` / `redact` / `outputs` / `mask` (the
engine ignores them). Only the plan decision may reshape inputs or outputs.

Library map keys are OPA module names, not paths. Do not use `inbound.rego` or
`outbound.rego`. A control-plane loader returns blobs from SQL or an API; it
does not need a directory layout.

Load-only loaders (no `SharedPolicySource`) keep today’s behavior.

## Filesystem adapter

`FilePolicyLoader` maps a tree onto the types above:

```text
{Dir}/
  _shared/
    inbound.rego           # optional, package shared.inbound
    outbound.rego          # optional, package shared.outbound
    lib/**/*.rego          # optional libraries (module name lib/<rel>)
  {planId}/{version}/
    inbound.rego
    outbound.rego
    data.json
```

`planId` `_shared` is rejected by this adapter only. Missing `_shared` or plan
files is not an error (nil bundle / nil shared).
