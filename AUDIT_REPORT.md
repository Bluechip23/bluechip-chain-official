# Bluechip Chain - Mainnet Launch Security Audit Report

**Date:** 2026-02-27
**Chain ID:** bluechip-1
**Cosmos SDK:** v0.50.13 | **CometBFT:** v0.38.17 | **IBC-Go:** v8.5.2 | **CosmWasm:** v0.54.0

---

## Executive Summary

This audit covers the Bluechip Chain codebase for mainnet launch readiness, including the custom `fixedmint` module, app-level configuration, genesis parameters, IBC security, CosmWasm configuration, and deployment infrastructure.

**Total Findings: 24**
| Severity | Count |
|----------|-------|
| Critical | 4 |
| High | 6 |
| Medium | 7 |
| Low | 7 |

**Verdict: NOT READY for mainnet launch.** There are 4 critical and 6 high-severity issues that must be resolved before launch.

---

## Critical Findings

### C-1: Hardcoded Mint Amount With No Governance Control

- **Severity:** Critical
- **Location:** `x/fixedmint/keeper/keeper.go:21`
- **Description:** The `MintFixedBlockReward` function hardcodes the mint amount to `1,000,000 ubluechip` (1 BLUECHIP) per block. This value is not stored as a module parameter and cannot be adjusted via governance.
- **Impact:** At ~5,256,000 blocks/year, this permanently mints ~5,256,000 BLUECHIP/year with no halving schedule, no supply cap, and no way to change the amount without a coordinated chain upgrade. If the token appreciates significantly, the community cannot reduce emissions. This creates unbounded perpetual inflation.
- **Vulnerable Code:**
```go
// x/fixedmint/keeper/keeper.go:20-35
func (k Keeper) MintFixedBlockReward(ctx context.Context) error {
    amount := sdk.NewCoins(sdk.NewCoin("ubluechip", math.NewInt(1000000))) // HARDCODED

    if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, amount); err != nil {
        return err
    }

    if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, authtypes.FeeCollectorName, amount); err != nil {
        return err
    }

    k.Logger().Info("Minted fixed block reward", "amount", amount)
    return nil
}
```
- **Recommendation:**
  1. Add `MintAmount` and `MintDenom` fields to the `Params` protobuf message
  2. Add validation in `Params.Validate()` to enforce safe ranges (e.g., max mint per block)
  3. Read the amount from params in `MintFixedBlockReward` instead of hardcoding
  4. Consider adding a `MaxSupplyCap` parameter to halt minting when reached
  5. Consider adding a halving schedule or epoch-based emission reduction

```go
// Suggested fix
func (k Keeper) MintFixedBlockReward(ctx context.Context) error {
    params := k.GetParams(ctx)
    if params.MintAmount.IsZero() {
        return nil // minting disabled
    }
    amount := sdk.NewCoins(sdk.NewCoin(params.MintDenom, params.MintAmount))
    // ... rest of function
}
```

---

### C-2: CosmWasm Unrestricted Code Upload on Mainnet

- **Severity:** Critical
- **Location:** `genesis.json:331-338`
- **Description:** CosmWasm code upload and contract instantiation permissions are set to `"Everybody"`, allowing any account to deploy arbitrary smart contracts on the chain.
- **Impact:** Malicious actors can deploy contracts that: drain funds via social engineering, perform token spam attacks, exploit gas quirks for DoS, or deploy honeypot/scam contracts. This is the single most exploited misconfiguration in CosmWasm chains.
- **Vulnerable Configuration:**
```json
"wasm": {
    "params": {
        "code_upload_access": {
            "permission": "Everybody",
            "addresses": []
        },
        "instantiate_default_permission": "Everybody"
    }
}
```
- **Recommendation:** Restrict to governance-only for mainnet launch. Can be relaxed later via governance proposal once the chain is stable:
```json
"wasm": {
    "params": {
        "code_upload_access": {
            "permission": "Nobody"
        },
        "instantiate_default_permission": "Nobody"
    }
}
```
Or use `"AnyOfAddresses"` with a curated allowlist of audited deployers.

---

### C-3: ICA Host Allows All Message Types (Wildcard)

- **Severity:** Critical
- **Location:** `genesis.json:323-326`
- **Description:** The Interchain Accounts host module is configured with `allow_messages: ["*"]`, permitting any SDK message type to be executed remotely via ICA.
- **Impact:** An attacker controlling an ICA controller on another chain could execute arbitrary governance proposals, staking operations, bank transfers, or even upgrade proposals on your chain. This effectively grants remote root access to any chain that establishes an ICA channel.
- **Vulnerable Configuration:**
```json
"host_genesis_state": {
    "params": {
        "host_enabled": true,
        "allow_messages": ["*"]
    }
}
```
- **Recommendation:** Restrict to only the message types you need:
```json
"allow_messages": [
    "/cosmos.bank.v1beta1.MsgSend",
    "/cosmos.staking.v1beta1.MsgDelegate",
    "/cosmos.staking.v1beta1.MsgUndelegate",
    "/ibc.applications.transfer.v1.MsgTransfer"
]
```

---

### C-4: No Genesis Validators Defined

- **Severity:** Critical
- **Location:** `genesis.json:232`
- **Description:** The `gen_txs` array is empty. Without genesis transactions (gentxs), the chain cannot produce blocks at genesis.
- **Impact:** The chain will fail to start. This is a hard blocker for launch.
- **Vulnerable Configuration:**
```json
"genutil": {
    "gen_txs": []
}
```
- **Recommendation:** Collect gentxs from your initial validator set using `bluechipchaind genesis gentx` and add them to the genesis file before launch. Ensure at least 1 validator (ideally 3+) has submitted a gentx.

---

## High Findings

### H-1: Minimum Gas Prices Not Set

- **Severity:** High
- **Location:** `config/app.toml:11`, `cmd/bluechipchaind/cmd/config.go:43`
- **Description:** The `minimum-gas-prices` is set to an empty string `""`. The config code explicitly comments out setting a default: `// srvCfg.MinGasPrices = "0stake"`.
- **Impact:** Validators accept zero-fee transactions by default, enabling transaction spam attacks that can fill blocks with garbage transactions, bloat state, and degrade chain performance. Especially dangerous with CosmWasm enabled, as contract execution can be computationally expensive.
- **Recommendation:** Set a non-zero minimum gas price in `cmd/bluechipchaind/cmd/config.go`:
```go
srvCfg.MinGasPrices = "0.025ubluechip"
```
And in `config/app.toml`:
```toml
minimum-gas-prices = "0.025ubluechip"
```

---

### H-2: Dual Minting Architecture Conflict

- **Severity:** High
- **Location:** `app/app_config.go:106-125` (beginBlockers order)
- **Description:** Both the standard Cosmos SDK `mint` module and the custom `fixedmint` module are registered as beginBlockers. While the `mint` module's inflation is currently set to 0%, governance can change this at any time, causing unintended double inflation.
- **Impact:** If governance passes a proposal to increase `mint` module inflation (perhaps not realizing `fixedmint` already handles emissions), the chain would experience double inflation - both dynamic (from `mint`) and fixed (from `fixedmint`). This could rapidly devalue the token.
- **Recommendation:** Either:
  1. **Remove the `mint` module entirely** if `fixedmint` is the intended sole inflation mechanism, OR
  2. **Add a circuit breaker** that disables `fixedmint` if `mint` inflation is >0, OR
  3. **Document this clearly** and add a governance guardrail that prevents setting `mint` inflation above 0

---

### H-3: fixedmint Module Has Excessive Permissions

- **Severity:** High
- **Location:** `app/app_config.go:165`
- **Description:** The fixedmint module account is registered with `Minter`, `Burner`, and `Staking` permissions, but the module only performs minting operations.
- **Impact:** If a bug or future code change allows unintended code paths, the module could burn tokens or perform staking operations. Following the principle of least privilege, unnecessary permissions increase the blast radius of any vulnerability.
- **Vulnerable Code:**
```go
{Account: fixedmintmoduletypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner, authtypes.Staking}},
```
- **Recommendation:** Remove unused permissions:
```go
{Account: fixedmintmoduletypes.ModuleName, Permissions: []string{authtypes.Minter}},
```

---

### H-4: API Write Timeout Disabled

- **Severity:** High
- **Location:** `config/app.toml:146`
- **Description:** `rpc-write-timeout = 0` disables the HTTP write timeout for the REST API server.
- **Impact:** Slow-read (Slowloris-style) DoS attacks can hold connections open indefinitely, exhausting the server's connection pool (max 1000 connections). An attacker could make the REST API completely unavailable with minimal resources.
- **Recommendation:**
```toml
rpc-write-timeout = 10
```

---

### H-5: No Upgrade Handler Registered

- **Severity:** High
- **Location:** `app/app.go` (entire file - no upgrade handler found)
- **Description:** No upgrade handlers are registered anywhere in the codebase. The upgrade module is imported but not configured with any planned upgrade paths.
- **Impact:** If a critical vulnerability is discovered post-launch, there is no mechanism for a coordinated chain upgrade. The chain would need to halt, and all validators would need to manually coordinate a binary swap, which is error-prone and slow.
- **Recommendation:** Before mainnet launch, register at least a placeholder upgrade handler:
```go
app.UpgradeKeeper.SetUpgradeHandler(
    "v1.1.0",
    func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
        return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
    },
)
```

---

### H-6: Telemetry and Monitoring Completely Disabled

- **Severity:** High
- **Location:** `config/app.toml:90`, `config/config.toml:486`
- **Description:** Both application telemetry (`enabled = false`) and Prometheus metrics (`prometheus = false`) are disabled.
- **Impact:** No observability into chain health, performance degradation, memory leaks, consensus issues, or attack patterns. Operators will be flying blind on mainnet, unable to detect and respond to issues before they become critical.
- **Recommendation:**
```toml
# app.toml
[telemetry]
enabled = true
prometheus-retention-time = 600

# config.toml
[instrumentation]
prometheus = true
```

---

## Medium Findings

### M-1: Coin Type 118 Conflicts With Cosmos Hub

- **Severity:** Medium
- **Location:** `app/app.go:93`
- **Description:** `ChainCoinType = 118` uses the same BIP-44 coin type as Cosmos Hub and many other Cosmos chains.
- **Impact:** Users with hardware wallets or HD wallets will derive the same addresses across Bluechip and Cosmos Hub (and other chains using 118). While the bech32 prefix differentiates display addresses, key derivation collision can cause user confusion and potential fund mismanagement.
- **Recommendation:** Register a unique coin type via the SLIP-0044 registry, or use an unregistered number (e.g., `9876`):
```go
ChainCoinType = 9876 // unique to Bluechip
```

---

### M-2: Empty Params - No Governance-Tunable Parameters

- **Severity:** Medium
- **Location:** `x/fixedmint/types/params.go:19-23`
- **Description:** The `Params` struct is completely empty with no fields. The `Validate()` function always returns nil. This means the fixedmint module has zero governance-tunable parameters.
- **Impact:** The chain cannot adapt its emission schedule, mint denom, or any other fixedmint behavior without a full binary upgrade. This severely limits the chain's ability to respond to economic conditions.
- **Recommendation:** Add meaningful parameters:
```go
func NewParams(mintAmount math.Int, mintDenom string, mintEnabled bool) Params {
    return Params{
        MintAmount:  mintAmount,
        MintDenom:   mintDenom,
        MintEnabled: mintEnabled,
    }
}
```

---

### M-3: No Module Invariants Registered

- **Severity:** Medium
- **Location:** `x/fixedmint/module/module.go:129`
- **Description:** `RegisterInvariants` is a no-op function that registers no invariants.
- **Impact:** There is no automated way to verify that the fixedmint module's state is consistent. For example, if a bug causes the module to mint incorrect amounts, or if state corruption occurs, it would go undetected by the crisis module.
- **Recommendation:** Register at least a total-minted-supply invariant that tracks cumulative minting and verifies it matches expectations.

---

### M-4: Evidence Max Age Duration Too Short

- **Severity:** Medium
- **Location:** `genesis.json:13`
- **Description:** `max_age_duration` is set to `172800000000000` nanoseconds (48 hours), while the unbonding period is 21 days (1,814,400 seconds).
- **Impact:** Evidence of validator misbehavior (e.g., double signing) older than 48 hours is rejected. A malicious validator could double-sign and simply wait 48 hours before the evidence is submitted, avoiding any penalty. The evidence window should match or exceed the unbonding period.
- **Recommendation:**
```json
"max_age_duration": "1814400000000000"
```
(21 days in nanoseconds, matching the unbonding period)

---

### M-5: Governance Min Initial Deposit Ratio is Zero

- **Severity:** Medium
- **Location:** `genesis.json:174`
- **Description:** `min_initial_deposit_ratio` is `"0.000000000000000000"`, meaning proposals can be created with zero initial deposit.
- **Impact:** Anyone can spam governance with zero-cost proposals, cluttering the governance interface and potentially confusing voters. Combined with a 2-day voting period, this could be used for social engineering attacks.
- **Recommendation:** Set to at least 10-25%:
```json
"min_initial_deposit_ratio": "0.250000000000000000"
```

---

### M-6: Release Workflow Triggers on Any Push

- **Severity:** Medium
- **Location:** `.github/workflows/release.yml:15`
- **Description:** The workflow trigger is `on: push` with no branch or tag filters.
- **Impact:** Every push to any branch triggers the release workflow, wasting CI resources and potentially creating unintended releases from feature branches.
- **Recommendation:**
```yaml
on:
  push:
    tags:
      - 'v*'
    branches:
      - main
```

---

### M-7: State Sync Snapshots Disabled

- **Severity:** Medium
- **Location:** `config/app.toml:194`
- **Description:** `snapshot-interval = 0` disables state sync snapshots.
- **Impact:** New validators and full nodes cannot use state sync to quickly bootstrap, forcing them to replay the entire block history from genesis. As the chain grows, this significantly increases time and cost to join the network.
- **Recommendation:**
```toml
snapshot-interval = 1000
snapshot-keep-recent = 2
```

---

## Low Findings

### L-1: Slashing Downtime Fraction Unusually High

- **Severity:** Low
- **Location:** `genesis.json:210`
- **Description:** `slash_fraction_downtime` is set to `0.010000000000000000` (1%), which is 100x higher than the Cosmos Hub standard of 0.01% (0.0001).
- **Impact:** Validators who experience brief downtime (e.g., routine server maintenance exceeding the signed_blocks_window) lose 1% of their delegated stake, which is disproportionately punitive and may discourage validator participation.
- **Recommendation:** Use a more standard value:
```json
"slash_fraction_downtime": "0.000100000000000000"
```

---

### L-2: Config Contains Developer Machine Moniker

- **Severity:** Low
- **Location:** `config/config.toml:22`
- **Description:** `moniker = "jeremywhitepc2"` - the node moniker contains a developer's personal machine name.
- **Impact:** Information leakage of developer identity. The moniker is visible on the P2P network and in explorer UIs.
- **Recommendation:** Change to a production-appropriate moniker before launch.

---

### L-3: No Seed Nodes or Persistent Peers

- **Severity:** Low
- **Location:** `config/config.toml:214-217`
- **Description:** Both `seeds` and `persistent_peers` are empty strings.
- **Impact:** Nodes cannot discover peers or bootstrap into the network. While this is expected for a pre-launch config template, it must be populated before distributing to validators.
- **Recommendation:** Configure seed nodes and initial persistent peers before distributing the genesis package.

---

### L-4: pprof Debug Endpoint Enabled

- **Severity:** Low
- **Location:** `config/config.toml:198`
- **Description:** `pprof_laddr = "localhost:6060"` enables the Go pprof profiling endpoint.
- **Impact:** While bound to localhost, if any reverse proxy or port forwarding exposes this, it leaks detailed runtime information (goroutine stacks, memory allocation, CPU profiles) that could aid attackers.
- **Recommendation:** Disable for production:
```toml
pprof_laddr = ""
```

---

### L-5: Query Gas Limit Unbounded

- **Severity:** Low
- **Location:** `config/app.toml:16`
- **Description:** `query-gas-limit = "0"` means queries can consume unlimited gas.
- **Impact:** Expensive queries (e.g., iterating all accounts, large contract state reads) can consume excessive node resources, potentially causing OOM or degraded performance.
- **Recommendation:**
```toml
query-gas-limit = "3000000"
```

---

### L-6: Double Sign Check Height Disabled

- **Severity:** Low
- **Location:** `config/config.toml:432`
- **Description:** `double_sign_check_height = 0` disables the double-sign safety check on validator restart.
- **Impact:** If a validator accidentally runs two instances with the same key, the node won't detect that it recently signed blocks and will proceed to double-sign, resulting in slashing (5% stake loss and permanent jailing).
- **Recommendation:**
```toml
double_sign_check_height = 10
```

---

### L-7: SDK App-Side Mempool Disabled

- **Severity:** Low
- **Location:** `config/app.toml:236`
- **Description:** `max-txs = -1` disables the SDK's app-side mempool, relying entirely on CometBFT's mempool.
- **Impact:** The SDK's app-side mempool provides additional transaction ordering and priority features. Disabling it is valid but means the chain cannot leverage advanced mempool features like priority ordering or lane-based transaction processing.
- **Recommendation:** Consider setting a positive value if you want priority-based transaction ordering:
```toml
max-txs = 5000
```

---

## Informational Notes

### I-1: Denom Hardcoded in Mint Function
The denom `"ubluechip"` is hardcoded in `keeper.go:21` rather than read from params. If the chain ever changes its base denom (unlikely but possible via migration), the fixedmint module would need a binary upgrade.

### I-2: Test Coverage is Minimal
Tests only cover basic MsgServer instantiation and UpdateParams. There are no tests for `MintFixedBlockReward`, no integration tests, and no simulation-based testing of the fixedmint module.

### I-3: Deprecated Dependencies
The `github.com/golang/protobuf` package (v1.5.4) is in maintenance mode. Consider migrating to `google.golang.org/protobuf`.

### I-4: config.yml Contains Test Accounts
The Ignite `config.yml` contains test accounts (alice, bob) with token allocations. Ensure this file is not used for mainnet genesis generation.

### I-5: Wasm Max Size is 30MB
`max-wasm-size = "30000000"` (30MB) is generous. Most CosmWasm contracts are under 1MB. Consider reducing to prevent large contract uploads.

---

## Pre-Launch Checklist

- [ ] **Fix all Critical findings (C-1 through C-4)**
- [ ] **Fix all High findings (H-1 through H-6)**
- [ ] Review and address Medium findings
- [ ] Populate genesis with validator gentxs
- [ ] Set minimum gas prices in default config
- [ ] Restrict CosmWasm upload permissions
- [ ] Restrict ICA allowed messages
- [ ] Make fixedmint amount governance-configurable
- [ ] Enable telemetry and monitoring
- [ ] Configure seed nodes and persistent peers
- [ ] Remove developer moniker from config
- [ ] Enable state sync snapshots
- [ ] Register at least one upgrade handler
- [ ] Run full simulation tests
- [ ] Perform a testnet dry-run with production genesis
- [ ] Have at least one external security audit firm review the fixedmint module

---

*This audit was performed via static analysis of the source code repository. It does not include dynamic testing, fuzzing, or formal verification. A professional security audit by a third-party firm is recommended before mainnet launch.*
