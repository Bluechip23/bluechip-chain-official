# Security Audit Report: BluechipChain

**Date:** 2026-02-27
**Scope:** Full Cosmos SDK blockchain and custom modules (`liquidityvault`, `fixedmint`)
**Chain version:** Cosmos SDK v0.50.13, wasmd v0.54.0, IBC-go v8.5.2

---

## Executive Summary

This audit covers the BluechipChain Cosmos SDK blockchain with two custom modules: `liquidityvault` (validator liquidity provisioning and composite scoring) and `fixedmint` (fixed block reward minting). The audit identified **4 Critical**, **6 High**, **9 Medium**, and **7 Low/Informational** findings. The most severe issues relate to missing authorization checks, a non-atomic deposit flow, an unenforced StakeCap parameter, and the absence of any fund withdrawal mechanism.

---

## Findings

### CRITICAL-01: Missing Signer Authorization in `DepositToVault` - Potential Unauthorized Fund Transfer

**Severity:** Critical
**Location:** `x/liquidityvault/keeper/msg_server.go:74-132`

**Description:**
The `DepositToVault` handler derives the sender address from `req.ValidatorAddress` and transfers funds from that address to the module account. The message handler does NOT verify that the transaction signer matches `req.ValidatorAddress`. Authorization depends entirely on the `cosmos.msg.v1.signer` proto annotation in the `.proto` file pointing to the correct field. If this annotation is misconfigured or missing, any user could drain funds from any validator's account.

```go
// Line 85: sender is derived from the request field, not verified against tx signer
senderAddr, err := sdk.AccAddressFromBech32(req.ValidatorAddress)
// Line 91: funds transferred from this unverified address
if err := k.bankKeeper.SendCoinsFromAccountToModule(sdkCtx, senderAddr, types.ModuleName, sdk.NewCoins(req.Amount0)); err != nil {
```

**Impact:** If the proto signer annotation does not point to `validator_address`, any account could submit a `MsgDepositToVault` referencing another account's address and steal their funds.

**Recommendation:**
1. Verify the `.proto` file has `option (cosmos.msg.v1.signer) = "validator_address"` on `MsgDepositToVault`
2. Add an explicit signer check in the handler as defense-in-depth:
```go
signerAddr := sdk.AccAddressFromBech32(req.ValidatorAddress)
if !signerAddr.Equals(sdk.MustAccAddressFromBech32(/* actual tx signer */)) {
    return nil, types.ErrInvalidSigner
}
```

---

### CRITICAL-02: Griefing Attack via Unrestricted `RegisterValidator` for LIQUIDITY Type

**Severity:** Critical
**Location:** `x/liquidityvault/keeper/msg_server.go:42-72`

**Description:**
The `RegisterValidator` handler allows anyone to register a vault with `ValidatorType_VALIDATOR_TYPE_LIQUIDITY` for any arbitrary `validator_address` string. There is no verification that the signer owns or controls the registered address. Once a vault is created, `ErrVaultAlreadyExists` prevents the legitimate owner from ever registering.

```go
// Line 42-56: For LIQUIDITY type, only UNSPECIFIED is rejected. No ownership check.
func (k msgServer) RegisterValidator(...) {
    if req.ValidatorType == types.ValidatorType_VALIDATOR_TYPE_UNSPECIFIED {
        return nil, types.ErrInvalidValidatorType
    }
    // For Full validators, verify staking registration (but NOT ownership)
    if req.ValidatorType == types.ValidatorType_VALIDATOR_TYPE_FULL {
        // ... staking check only
    }
    // Creates vault for ANY address without signer verification
    if err := k.CreateVault(goCtx, req.ValidatorAddress, req.ValidatorType); err != nil {
```

**Impact:**
- An attacker can register vaults for all known validator addresses, permanently blocking them from the liquidityvault system
- The attacker can register FULL-type vaults for addresses that are actually LIQUIDITY validators, locking them into the wrong type
- No `DeleteVault` or `UpdateValidatorType` mechanism exists to fix this

**Recommendation:**
1. Require the signer to be the `validator_address` owner (verify proto signer annotation)
2. For FULL type, also verify the signer controls the validator operator address
3. Consider adding governance-gated vault deletion for recovery

---

### CRITICAL-03: No Withdrawal Mechanism - Deposited Funds Are Permanently Locked

**Severity:** Critical
**Location:** `x/liquidityvault/keeper/msg_server.go` (entire file) and `x/liquidityvault/types/` (no `MsgWithdraw` defined)

**Description:**
The module provides `MsgDepositToVault` to deposit funds into pool contracts via the module account, but there is no corresponding withdrawal, claim, or removal message. Once ubluechip tokens are sent to the module account and deposited into a pool, there is no on-chain mechanism to:
- Withdraw liquidity positions from pools
- Reclaim tokens from the module account
- Remove or close positions

The registered codec types confirm only 4 messages exist: `MsgUpdateParams`, `MsgRegisterValidator`, `MsgDepositToVault`, `MsgSetDelegatorRewardPercent`.

**Impact:** All funds deposited through this module are permanently locked. Validators who deposit into vaults will lose those funds with no recovery path.

**Recommendation:**
1. Implement `MsgWithdrawFromVault` with appropriate authorization checks
2. Implement position removal from vault state
3. Consider emergency governance-gated withdrawal for edge cases

---

### CRITICAL-04: StakeCap Parameter Defined But Never Enforced

**Severity:** Critical
**Location:** `x/liquidityvault/types/params.go:14`, `x/liquidityvault/keeper/msg_server.go:74-132`

**Description:**
The `StakeCap` parameter is defined with a default of 1,000,000,000,000 ubluechip (1M BLUECHIP) and has validation in `Params.Validate()`. However, it is never checked or enforced anywhere in the codebase. The `DepositToVault` handler does not check if the deposit would exceed the stake cap. The `ErrStakeCapExceeded` error is defined but never returned.

```go
// types/params.go:14 - defined
DefaultStakeCap = math.NewInt(1_000_000_000_000)

// types/errors.go:16 - error defined but unused
ErrStakeCapExceeded = sdkerrors.Register(ModuleName, 1104, "delegation would exceed stake cap")

// msg_server.go:74-132 - DepositToVault has NO stake cap check
```

**Impact:** The entire stake cap mechanism is non-functional. Any validator can deposit unlimited amounts, defeating the purpose of the cap (likely intended to prevent stake centralization).

**Recommendation:**
Add a stake cap check in `DepositToVault`:
```go
if vault.TotalDeposited.Add(req.Amount0).Amount.GT(params.StakeCap) {
    return nil, types.ErrStakeCapExceeded
}
```

---

### HIGH-01: Non-Atomic Deposit Flow - Coins Trapped on Wasm Failure

**Severity:** High
**Location:** `x/liquidityvault/keeper/msg_server.go:90-105`

**Description:**
The `DepositToVault` handler performs three sequential operations:
1. Transfer ubluechip from sender to module account (line 91)
2. Execute wasm deposit to pool contract (line 96)
3. Update vault state (line 116)

If step 2 (wasm execution) fails, the Cosmos SDK transaction will revert the entire state change (including the bank transfer), which is correct behavior due to the cache-multistore transaction pattern. **However**, if step 2 succeeds but step 3 fails (e.g., `SetVault` returns an error), the wasm execution state changes are also reverted but the position may have been created in the contract's internal state depending on the contract implementation.

More importantly, the wasm `Execute` call on line 62 sends `amount0` (ubluechip) as native funds attached to a CW20 Send message to the CW20 contract. This means the ubluechip goes to the CW20 contract, not the pool contract. This is architecturally suspicious - the CW20 contract receives native tokens alongside a CW20 transfer hook.

```go
// Line 62: Sends ubluechip to CW20 contract, not pool contract
_, err = k.wasmKeeper.Execute(sdkCtx, cw20Addr, moduleAddr, cw20SendBz, sdk.NewCoins(amount0))
```

**Impact:** Depending on the CW20 contract behavior, native ubluechip tokens sent alongside CW20 Send messages may be lost or misrouted if the contract does not forward them to the pool.

**Recommendation:**
1. Verify the CW20 contract correctly forwards native tokens in the Send hook
2. Consider separating the native token deposit from the CW20 send into two operations
3. Add verification queries after deposit to confirm the position was created correctly

---

### HIGH-02: Custom Base64 Implementation Instead of Standard Library

**Severity:** High
**Location:** `x/liquidityvault/keeper/wasm.go:220-252`

**Description:**
The module implements its own base64 encoding function (`encodeBase64`) instead of using Go's well-tested `encoding/base64` standard library package. Custom cryptographic/encoding implementations are a known source of subtle bugs.

```go
// Lines 220-252: Hand-rolled base64 encoding
func encodeBase64(data []byte) string {
    const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
    // ... 30+ lines of manual bit manipulation
```

While the implementation appears functionally correct on inspection, it:
- Has no test coverage for edge cases (empty input, single byte, two bytes, exact multiple of 3)
- Could diverge from standard base64 behavior in subtle ways
- Makes code review and maintenance harder

**Impact:** If the encoding produces incorrect output, wasm contract calls will silently fail or produce unexpected behavior. This is a blockchain consensus-critical code path.

**Recommendation:**
Replace with standard library:
```go
import "encoding/base64"
func encodeBase64(data []byte) string {
    return base64.StdEncoding.EncodeToString(data)
}
```

---

### HIGH-03: Empty `minimum-gas-prices` Allows Zero-Fee Transaction Spam

**Severity:** High
**Location:** `config/app.toml:11`

**Description:**
The node configuration has `minimum-gas-prices = ""` (empty string). While the comment states validators should configure this themselves, shipping with an empty default means any operator using default configuration is vulnerable to transaction spam attacks with zero fees.

**Impact:** Attackers can flood the mempool with zero-fee transactions, causing network congestion and potential DoS against validators.

**Recommendation:**
Set a non-zero default: `minimum-gas-prices = "0.001ubluechip"`

---

### HIGH-04: Unbounded Query Gas Limit

**Severity:** High
**Location:** `config/app.toml:15`

**Description:**
`query-gas-limit = "0"` means queries have no gas limit. Combined with unbounded state iteration (see MEDIUM-04), this allows attackers to craft expensive queries that consume unbounded node resources.

**Impact:** DoS via expensive RPC queries that consume unlimited CPU/memory on full nodes.

**Recommendation:**
Set `query-gas-limit = "3000000"` or similar reasonable bound.

---

### HIGH-05: Hardcoded Inflationary Mint Amount With No Governance Control

**Severity:** High
**Location:** `x/fixedmint/keeper/keeper.go:20-35`

**Description:**
The `fixedmint` module mints exactly 1,000,000 ubluechip (1 BLUECHIP) per block and sends it to the fee collector. This amount is hardcoded with no governance parameter to adjust it.

```go
// Line 21: Hardcoded mint amount
amount := sdk.NewCoins(sdk.NewCoin("ubluechip", math.NewInt(1000000)))
```

At ~6 second blocks, this mints ~14,400 BLUECHIP per day (~5.26M per year), permanently inflating the supply with no ability to adjust, pause, or halt minting.

**Impact:**
- Cannot adjust inflation rate via governance
- Cannot halt minting in emergencies
- Combined with the standard Cosmos SDK `mint` module (also running), creates dual inflation sources
- No cap on total supply

**Recommendation:**
1. Move the mint amount to a governance-controllable parameter
2. Consider adding a total supply cap
3. Evaluate interaction with the standard `mint` module inflation

---

### HIGH-06: Missing Address Validation for LIQUIDITY-Type Validators

**Severity:** High
**Location:** `x/liquidityvault/keeper/msg_server.go:42-72`, `x/liquidityvault/keeper/vault.go:70-90`

**Description:**
When registering a `VALIDATOR_TYPE_LIQUIDITY` validator, the `validator_address` field is never validated as a proper bech32 address. Only `VALIDATOR_TYPE_FULL` validators go through `sdk.ValAddressFromBech32()` validation. The `CreateVault` function in vault.go also performs no address validation.

```go
// msg_server.go - LIQUIDITY type skips ALL address validation
if req.ValidatorType == types.ValidatorType_VALIDATOR_TYPE_FULL {
    valAddr, err := sdk.ValAddressFromBech32(req.ValidatorAddress)  // Only for FULL
    // ...
}
// CreateVault called with unvalidated string for LIQUIDITY type
if err := k.CreateVault(goCtx, req.ValidatorAddress, req.ValidatorType); err != nil {
```

**Impact:**
- Arbitrary strings can be stored as validator addresses
- Could cause downstream parsing failures in other operations
- Key space pollution with invalid entries
- Makes it harder to distinguish legitimate entries from malicious ones

**Recommendation:**
Add bech32 validation for all validator types in `RegisterValidator`:
```go
if _, err := sdk.AccAddressFromBech32(req.ValidatorAddress); err != nil {
    return nil, fmt.Errorf("invalid validator address: %w", err)
}
```

---

### MEDIUM-01: Unbounded Vault Positions Array

**Severity:** Medium
**Location:** `x/liquidityvault/keeper/msg_server.go:109-114`

**Description:**
Each deposit appends a new `PoolPosition` to the vault's `Positions` slice with no upper bound. There is no limit on the number of positions a single vault can hold.

```go
vault.Positions = append(vault.Positions, types.PoolPosition{...})
```

**Impact:**
- A validator making many small deposits inflates the serialized vault size
- BeginBlock value post execution iterates ALL positions per vault (wasm.go:202), making it O(vaults * positions)
- Could cause block timeouts if a vault accumulates thousands of positions
- Increases state bloat and gas costs for all vault operations

**Recommendation:**
1. Add a `MaxPositionsPerVault` parameter (e.g., 50)
2. Consider consolidating positions for the same pool contract

---

### MEDIUM-02: BeginBlock Errors Are Silently Swallowed

**Severity:** Medium
**Location:** `x/liquidityvault/keeper/keeper.go:78-112`

**Description:**
All three BeginBlock operations (value posts, simple checks, complex checks) catch and log errors but never return them. Failed operations silently continue, potentially leaving the module in an inconsistent state.

```go
// Lines 85-87: Error logged but BeginBlock returns nil
if err := k.ExecuteValuePost(ctx); err != nil {
    k.Logger().Error("failed to execute value post", "error", err)
}
```

**Impact:**
- Persistent wasm query failures could cause all value posts to silently use fallback deposit values, making composite scores inaccurate
- Failed checks could leave stale rankings indefinitely
- Hard to detect and diagnose issues in production

**Recommendation:**
At minimum, emit error events that can be monitored. Consider returning errors for consensus-critical operations.

---

### MEDIUM-03: Deterministic and Predictable Value Post Scheduling

**Severity:** Medium
**Location:** `x/liquidityvault/keeper/value_posts.go:82-127`

**Description:**
Value post scheduling uses the block header hash as an RNG seed. This hash is identical for all validators at the same block height, making the schedule deterministic and predictable. All validators will agree on the schedule (necessary for consensus), but external observers can also predict exactly which blocks will trigger value posts.

```go
// Line 93-98: Block hash is public knowledge
headerHash := sdkCtx.HeaderHash()
seed = int64(binary.BigEndian.Uint64(headerHash[:8]))
rng := rand.New(rand.NewSource(seed))
```

**Impact:** If value post timing matters for economic security (e.g., to prevent manipulation of vault values before scoring), predictability allows validators to manipulate pool states just before known value post blocks.

**Recommendation:**
1. Document that value post timing is public information (if acceptable)
2. If timing sensitivity matters, consider using commit-reveal or VRF-based scheduling
3. Consider using multiple block hashes or other entropy sources

---

### MEDIUM-04: No Pagination in Query Endpoints

**Severity:** Medium
**Location:** `x/liquidityvault/keeper/query_vault.go:27-42`, `x/liquidityvault/keeper/query_score.go`

**Description:**
The `AllVaults` and `ValidatorRankings` query handlers return all records without pagination support. While `PageResponse` is constructed, no actual page filtering is applied.

```go
// query_vault.go:32 - Returns ALL vaults regardless of pagination request
vaults := k.GetAllVaults(goCtx)
pageRes := &query.PageResponse{
    Total: uint64(len(vaults)),
}
```

**Impact:** With hundreds or thousands of validators, these queries could:
- Time out on RPC nodes
- Consume excessive memory (especially combined with HIGH-04: unbounded query gas)
- Be used as a DoS vector against public RPC endpoints

**Recommendation:**
Implement proper `query.Paginate()` using the KV store iterator, or apply page offset/limit to the in-memory results.

---

### MEDIUM-05: Integer Precision Loss in Position Value Calculation

**Severity:** Medium
**Location:** `x/liquidityvault/keeper/wasm.go:152`

**Description:**
The position value calculation uses integer division, which truncates fractional results:

```go
posValue := liquidity.Mul(reserve0).Quo(totalLiquidity).Add(unclaimedFees0)
```

For example, if `liquidity=100`, `reserve0=999`, `totalLiquidity=1000`:
- Exact: 99.9 ubluechip
- Computed: 99 ubluechip (truncated)

**Impact:** Systematic undervaluation of positions. Over many positions and value posts, this compounds and produces consistently lower vault values than actual, affecting composite score accuracy and validator rankings.

**Recommendation:**
Consider using `math.LegacyDec` for intermediate calculations and rounding at the final step, or multiply by a precision factor before division.

---

### MEDIUM-06: Silent Fallback to Deposit Amount in Value Queries

**Severity:** Medium
**Location:** `x/liquidityvault/keeper/wasm.go:198-218`, `x/liquidityvault/keeper/composite_score.go:79-84`

**Description:**
When wasm queries fail (pool contract unreachable, migrated, or broken), the code silently falls back to using the original deposit amount as the position value. This happens in both `QueryTotalVaultValue` and `CalculateCompositeScore`.

```go
// wasm.go:210-211: Silent fallback on query failure
totalValue = totalValue.Add(pos.DepositAmount0)
continue

// composite_score.go:81-84: Silent fallback when no value posts
vault, found := k.GetVault(ctx, valAddr)
if found {
    score.VaultValue = vault.TotalDeposited.Amount
}
```

**Impact:**
- If a pool contract is exploited and drained, the vault value would still show the original deposit amount, masking a total loss
- Stale fallback values could persist indefinitely, misleading the ranking system
- No events or metrics to detect when fallbacks are being used

**Recommendation:**
1. Emit distinct events when fallback values are used
2. Consider marking vaults as "stale" if value queries fail for multiple consecutive attempts
3. Consider a maximum staleness threshold after which fallback values expire

---

### MEDIUM-07: Unsafe Byte-to-Enum Cast for ValidatorType

**Severity:** Medium
**Location:** `x/liquidityvault/keeper/vault.go:105`

**Description:**
`GetValidatorType` reads a single byte from the store and directly casts it to a `ValidatorType` enum without bounds checking:

```go
return types.ValidatorType(bz[0]), true
```

**Impact:** If store data is corrupted (disk error, migration bug, etc.), this could produce invalid enum values that cause unexpected behavior in downstream logic. In Go, enum types are just integers, so invalid values will silently pass type checks.

**Recommendation:**
```go
valType := types.ValidatorType(bz[0])
if _, ok := types.ValidatorType_name[int32(valType)]; !ok {
    return types.ValidatorType_VALIDATOR_TYPE_UNSPECIFIED, false
}
return valType, true
```

---

### MEDIUM-08: `MustUnmarshal` Panics on Store Corruption

**Severity:** Medium
**Location:** `x/liquidityvault/keeper/vault.go:23,52`, `x/liquidityvault/keeper/composite_score.go:35`, `x/liquidityvault/keeper/value_posts.go:43`

**Description:**
Multiple store read operations use `k.cdc.MustUnmarshal()`, which panics on unmarshal failure. If the protobuf schema changes without a proper migration, or if store data is corrupted, this will crash the node and potentially halt the chain.

```go
k.cdc.MustUnmarshal(bz, &vault)  // Panics on failure
```

**Impact:** Store corruption or schema mismatch causes chain halt (all nodes crash at the same block).

**Recommendation:**
Use non-panicking `k.cdc.Unmarshal()` with error handling. Log corrupted entries and skip them rather than crashing:
```go
if err := k.cdc.Unmarshal(bz, &vault); err != nil {
    k.Logger().Error("failed to unmarshal vault", "key", valAddr, "error", err)
    return types.Vault{}, false
}
```

---

### MEDIUM-09: Key Construction Without Length Prefix

**Severity:** Medium
**Location:** `x/liquidityvault/types/keys.go:44-78`

**Description:**
Store keys are constructed by appending raw validator address bytes to a prefix without a length prefix delimiter:

```go
func VaultKey(valAddr string) []byte {
    return append(VaultKeyPrefix, []byte(valAddr)...)
}
```

Since validator addresses are variable-length bech32 strings, there is a theoretical key collision risk if one address is a prefix of another. In practice, bech32 addresses have fixed lengths per address type, but this pattern is fragile and deviates from Cosmos SDK best practices which use length-prefixed keys.

**Impact:** Low practical risk with bech32 addresses, but a latent vulnerability if the addressing scheme changes. The `ValuePostKey` function is more concerning as it concatenates `valAddr + "/" + blockHeight`, where a crafted address containing "/" could cause ambiguity.

**Recommendation:**
Use length-prefixed keys following Cosmos SDK conventions:
```go
func VaultKey(valAddr string) []byte {
    addrBytes := []byte(valAddr)
    key := append(VaultKeyPrefix, byte(len(addrBytes)))
    return append(key, addrBytes...)
}
```

---

### LOW-01: Unbounded Iteration in BeginBlock Operations

**Severity:** Low (currently; escalates with validator count)
**Location:** `x/liquidityvault/keeper/value_posts.go:156-175`, `x/liquidityvault/keeper/validator_checks.go:21-38,66-83`

**Description:**
`ExecuteValuePost`, `ExecuteSimpleCheck`, and `ExecuteComplexCheck` all iterate over every vault via `IterateVaults`. For each vault, `QueryTotalVaultValue` queries every position (each requiring a wasm contract query). This is O(vaults * positions * wasm_queries) per check.

**Impact:** With many validators and positions, BeginBlock execution time grows unboundedly. At scale, this could cause blocks to exceed the timeout, leading to chain stalls.

**Recommendation:**
1. Add gas metering or time limits to BeginBlock operations
2. Consider processing vaults in batches across multiple blocks
3. Add a maximum vault/position count parameter

---

### LOW-02: Integer Overflow Risk in Median Calculation

**Severity:** Low
**Location:** `x/liquidityvault/keeper/composite_score.go:109`

**Description:**
The median calculation for even-length arrays adds two `math.Int` values before dividing by 2:

```go
return values[mid-1].Add(values[mid]).Quo(math.NewInt(2))
```

**Impact:** If both middle values are near `math.Int` maximum (2^256), the addition could overflow. In practice, vault values should be far below this threshold, making this theoretical.

**Recommendation:** Consider averaging without overflow: `a + (b - a) / 2` instead of `(a + b) / 2`.

---

### LOW-03: `SetDelegatorRewardPercent` Has No Ownership Check

**Severity:** Low
**Location:** `x/liquidityvault/keeper/msg_server.go:134-160`

**Description:**
The `SetDelegatorRewardPercent` handler allows setting the reward percentage on any vault without explicitly verifying the caller owns the vault. Like CRITICAL-01, this depends on the proto signer annotation being correctly set.

**Impact:** Same authorization concern as CRITICAL-01, though less severe as it only changes a percentage rather than transferring funds.

**Recommendation:** Add explicit ownership verification as defense-in-depth.

---

### LOW-04: Dual Inflation Sources

**Severity:** Low
**Location:** `app/app_config.go` (module ordering), `x/fixedmint/keeper/keeper.go:20-35`

**Description:**
The chain runs both the standard Cosmos SDK `mint` module (which implements dynamic inflation based on bonding ratio) AND the custom `fixedmint` module (which mints 1M ubluechip per block). Both run in BeginBlock, with `mint` executing before `fixedmint`.

**Impact:** The total inflation rate is the sum of both modules, making it harder to reason about token economics. The standard mint module's inflation targeting algorithm does not account for the fixedmint supply increase.

**Recommendation:** Either disable the standard `mint` module or document the intended interaction between the two inflation sources.

---

### LOW-05: ClearAllValuePosts Collects All Keys Before Deleting

**Severity:** Low
**Location:** `x/liquidityvault/keeper/value_posts.go:67-80`

**Description:**
The `ClearAllValuePosts` function collects all keys into an in-memory slice before deleting them. This is a common pattern to avoid iterator invalidation, but with many value posts, it could consume significant memory.

```go
keys := [][]byte{}
for ; iter.Valid(); iter.Next() {
    keys = append(keys, iter.Key())
}
for _, key := range keys {
    store.Delete(key)
}
```

**Impact:** Memory spike during complex check intervals proportional to the number of stored value posts.

**Recommendation:** Consider using a bounded batch delete pattern or the store's built-in prefix delete if available.

---

### LOW-06: No Rate Limiting on Vault Registration or Deposits

**Severity:** Low
**Location:** `x/liquidityvault/keeper/msg_server.go:42-132`

**Description:**
There is no rate limiting or cooldown period for registering vaults or making deposits. While gas costs provide some protection, this allows rapid-fire operations that could bloat state.

**Impact:** State bloat via rapid vault registration or deposit spam.

**Recommendation:** Consider adding a minimum deposit amount or registration fee.

---

### LOW-07: `liquidityvault` BankKeeper Interface Includes `MintCoins` But Module Only Has Burner Permission

**Severity:** Informational
**Location:** `x/liquidityvault/types/expected_keepers.go:24`, `app/app_config.go:173`

**Description:**
The `BankKeeper` interface for liquidityvault includes `MintCoins`, but the module account only has `Burner` permission. Any call to `MintCoins` would fail at runtime. Currently no code calls `MintCoins` on the liquidityvault module, but the interface suggests it was considered.

```go
// expected_keepers.go:24
MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error

// app_config.go:173 - Only Burner permission
{Account: liquidityvaultmoduletypes.ModuleName, Permissions: []string{authtypes.Burner}},
```

**Impact:** No current impact, but the unused capability in the interface could lead to confusion or accidental use.

**Recommendation:** Remove `MintCoins` from the BankKeeper interface if minting is not intended.

---

## Configuration Findings

### CONFIG-01: CosmWasm Permissions Set to "Everybody"

**Severity:** High (pre-mainnet)
**Location:** Genesis configuration / wasm module params

**Description:** The wasm module is configured with `instantiate_default_permission: "Everybody"` and open code upload permissions. This means any user can deploy and instantiate smart contracts.

**Recommendation:** For mainnet, restrict to governance-only or a whitelist: `"Nobody"` with governance proposals for uploads.

### CONFIG-02: ICA Host allow_messages Includes Wildcard

**Severity:** Medium (pre-mainnet)
**Location:** Genesis configuration / ICA host params

**Description:** If ICA host `allow_messages` is set to `["*"]`, any IBC-connected chain could execute arbitrary messages on this chain through interchain accounts.

**Recommendation:** Restrict to specific, reviewed message types.

### CONFIG-03: No-Op Mempool Configuration

**Severity:** Informational
**Location:** `config/app.toml`

**Description:** `max-txs = -1` configures a no-op mempool (CometBFT default handles mempool). This is standard for CometBFT v0.38+ but should be documented.

---

## Architecture Strengths

1. **Proper ante handler chain** with 17 decorators including circuit breaker, wasm gas limiting, and IBC redundant relay protection
2. **Module permission separation** - liquidityvault only has Burner, fixedmint has Minter+Burner+Staking (appropriate for their roles)
3. **Blocked module accounts** properly prevent direct transfers to critical module accounts
4. **IBC fee middleware** wrapping all IBC routes ensures relayer incentivization
5. **WasmKeeper adapter pattern** correctly separates permissioned execution from query access
6. **Phase 1 advisory mode** for validator checks is a prudent staged rollout approach
7. **Current dependency versions** - Cosmos SDK v0.50.13, wasmd v0.54.0, IBC-go v8.5.2 are all recent stable releases

---

## Summary by Severity

| Severity | Count | Key Themes |
|----------|-------|------------|
| Critical | 4 | Authorization, locked funds, unenforced parameters |
| High | 6 | Non-atomic operations, custom crypto, DoS vectors, no governance controls |
| Medium | 9 | Precision loss, unbounded state, silent fallbacks, key construction |
| Low | 7 | Scalability, rate limiting, interface hygiene |
| Config | 3 | Pre-mainnet hardening needed |

---

## Recommended Priority Actions

### Immediate (Pre-launch blockers)
1. Verify proto signer annotations for all message types (CRITICAL-01, CRITICAL-02)
2. Implement withdrawal mechanism (CRITICAL-03)
3. Enforce StakeCap in DepositToVault (CRITICAL-04)
4. Replace custom base64 with `encoding/base64` (HIGH-02)
5. Set minimum gas prices and query gas limit (HIGH-03, HIGH-04)
6. Add address validation for LIQUIDITY type (HIGH-06)
7. Restrict wasm upload permissions (CONFIG-01)

### Before Mainnet
8. Add pagination to query endpoints (MEDIUM-04)
9. Parameterize fixedmint amount (HIGH-05)
10. Add position limits per vault (MEDIUM-01)
11. Restrict ICA host allowed messages (CONFIG-02)

### Post-Launch Improvements
12. Replace MustUnmarshal with error-handling unmarshal (MEDIUM-08)
13. Add BeginBlock execution monitoring/limits (LOW-01)
14. Add rate limiting or minimum deposits (LOW-06)
