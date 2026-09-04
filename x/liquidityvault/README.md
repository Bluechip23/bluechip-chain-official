# x/liquidityvault

Stage 1 of the **Liquidity Providing Validators (LPV)** upgrade described in
the design document at the repository root (`Liquidity Providing Validators
(1).pdf`, "How Do We Get There? → Introduce liquidity vaults").

It introduces the three stage-1 concepts:

- **Liquidity Vault** — a per-validator pool of capital, held by the module
  account, that the validator commits to providing liquidity to pools in the
  BlueChip network.
- **Composite score** — the validator's tokens staked directly to the chain
  plus its active vault balance. Stage 1 exposes the score as a query; the
  simple/complex validator-set checks that consume it arrive in a later
  stage.
- **Stake cap** — a governance-set hard limit on how much can be bonded
  directly to any one validator. Capital above the cap is expected to flow
  into the Liquidity Vault instead.

Per the design document, the **only parameter a validator can change in
stage 1 is the fraction of vault rewards passed through to its delegators**
(`MsgSetRewardShare`). Withdrawal timing is protocol-wide, not per validator.

## Messages

| Message | Signer | Effect |
|---|---|---|
| `MsgDeposit` | validator operator | Moves bond-denom tokens from the validator's own account into its vault. Delegated funds cannot be deposited. |
| `MsgInitiateWithdrawal` | validator operator | Removes the amount from the active balance immediately (it stops counting toward the composite score) and releases it after the universal grace period. |
| `MsgSetRewardShare` | validator operator | Sets the delegator reward share, a fraction in [0, 1]. Defaults to 0.5. |
| `MsgAllocateToPool` | validator operator | Moves active vault balance into a registered pool; the position counts toward the composite score at current value. |
| `MsgDeallocateFromPool` | validator operator | Queues removal of pool liquidity; after the universal deallocation grace period the proceeds go to the validator's own account, leaving the vault (per the design document, withdrawn pool tokens must be re-staked or re-deposited manually). |
| `MsgCollectPoolRewards` | validator operator (with a position in the pool) | Pulls the pool's accrued fees and distributes them to every validator in the pool pro rata by shares. |
| `MsgClaimVaultRewards` | delegator | Pays out the delegator's accrued vault rewards from one validator. |
| `MsgRegisterPool` | governance | Registers a pool contract implementing the Vault Adapter Interface. |
| `MsgSetPoolEnabled` | governance | Enables/disables new allocations to a pool (deallocations always work). |
| `MsgUpdateParams` | governance | Sets the stake cap and the grace periods. |

## Parameters

| Param | Default | Meaning |
|---|---|---|
| `stake_cap` | `0` (disabled) | Maximum bond-denom tokens a validator may have bonded directly to the chain. Governance enables it by setting a positive value. |
| `withdrawal_grace_period` | `72h` | Universal delay between a vault withdrawal request and the release of funds. |
| `deallocation_grace_period` | `72h` | Universal delay ("Liquidity Providing Change") between a pool deallocation request and the liquidity leaving the pool. |
| `value_post_interval` | `20h` | Average time between a vault's value posts (six per five-day window). |

## Pool integration: the Vault Adapter Interface

The creator pools are CosmWasm contracts; the vault module deliberately does
not speak their native schema (two-sided deposits, CW20 mechanics, position
NFTs). Instead a registered pool contract must implement a minimal JSON
interface — see `types/wasm.go` for the schema:

- `provide_liquidity` (execute, bond-denom funds attached): add the funds to
  the caller's position, handling any swap/zap internally.
- `withdraw_liquidity {ratio}` (execute): remove that fraction of the
  caller's position and return the proceeds to the caller as bond-denom
  native funds in the same execution.
- `collect_rewards` (execute): send the accrued liquidity fees for the
  caller's position to the caller as bond-denom native funds in the same
  execution.
- `position_value {address}` (query): the address's position value in the
  bond denom.

The module account is the caller for every pool, holding one aggregate
position per pool at the contract. Validators own internal shares of it
(ERC-4626-style: first allocation mints 1:1, later allocations mint
`delta * total_shares / position_value` from the MEASURED value delta), so
the contracts need no per-validator accounting. Ratio truncation dust from
partial withdrawals stays in the position, favoring remaining shareholders.

Two share-math attacks are explicitly defended (see `AllocateToPool`):
zero-share mints are rejected, and any mint whose truncation loss would
exceed 0.1% of the allocation's measured value is rejected — so the classic
ERC-4626 donation/inflation attack (dust first allocation + donating into
the pool position to inflate the share price) fails the victim's transaction
harmlessly instead of skimming their deposit. The attacker's donation is
recoverable, so the residual is only a griefing vector (an inflated share
price rejects small allocations to that pool); governance ends it by
disabling the pool with `MsgSetPoolEnabled`.

A thin adapter contract wrapping the existing creator pools (implementing
the three calls above) lives on the contract side — it is the
`bluechip-contracts` repository's half of this integration.

## Value posts and the median composite score

Per the design document, a vault's contribution to the composite score is
the **median of six value posts** taken at pseudo-random times, damping both
market swings and post-timing gaming:

- The end blocker snapshots each vault's total value (active balance +
  valued positions) on its own cadence: every `value_post_interval` (default
  20h — six posts per five-day complex-check window) with a deterministic
  jitter in [interval/2, interval*3/2) derived from the block header hash
  and the validator address, so all nodes agree but validators cannot game
  post times far ahead.
- The last six posts are retained; `CompositeScore.composite_score` is
  `staked_tokens + median(posts)`. Before a vault's first post, the live
  vault value is used.
- If a pool contract cannot be queried during a post, the pool's last
  successfully observed value is used (zero if none was ever observed) — a
  broken pool degrades a score gracefully instead of halting the chain or
  zeroing the vault.

## Reward flow (F1-lite)

Any validator with a position in a pool can trigger `MsgCollectPoolRewards`;
the collected fees are split across the pool's validators pro rata by
internal shares, then each cut is split by that vault's
`delegator_reward_share`:

- The validator's part goes to the operator account immediately.
- The delegators' part raises the vault's cumulative **reward index**
  (`delegator_cut / validator_staked_tokens`). A delegator's entitlement is
  `(index_now − index_at_last_settlement) × their_stake`, settled lazily by
  the delegation hooks BEFORE any stake change — so distribution never loops
  over delegators, and a delegation created after a collection earns nothing
  from it. `MsgClaimVaultRewards` pays the floor of the accrual; fractional
  dust stays recorded.
- A validator with zero staked tokens (a pure liquidity validator) has no
  delegators to distribute to and receives its whole cut.
- All roundings favor the module, and slashing (which shrinks stake without
  a hook) can only lower settlements — the module can never owe more than it
  holds, which the module-account invariant now also enforces including
  outstanding rewards.

## Shadow validator-set ranking

The `SetRanking` query implements the design document's complex check —
validators ordered by staked tokens with the composite score as tiebreaker —
in **shadow mode**: it is observability only and has no effect on the actual
validator set. Wiring it into consensus-power selection is deliberately left
for a later, explicitly-approved stage; with the stake cap active, staking's
own top-N-by-bonded-tokens selection already enforces the simple check's
semantics continuously.

### End-blocker safety

Matured deallocations execute pool contracts inside the end blocker. Each
entry runs in a cached context under a bounded gas meter with a panic guard:
a failing or misbehaving pool contract can only fail its own entry, which is
requeued with a delay (1h) — it can never halt the chain.

## Stake cap enforcement

The cap is enforced through **staking hooks**, not an ante decorator, so every
delegation path is covered — including messages nested inside `authz`
`MsgExec`. The `Before*` delegation hooks snapshot the validator's tokens and
`AfterDelegationModified` rejects the operation if it *increased* the tokens
beyond the cap:

- Undelegations are never blocked, even for a validator already above a newly
  lowered cap.
- An operation that leaves tokens unchanged is never blocked.
- Paths that skip the `Before*` hooks (genesis import) are not enforced, so a
  cap lowered below existing stakes can never brick state import.

## End blocker

Matured pending withdrawals (grace period elapsed) are paid out from the
module account to the validator's operator account.

## Invariant

`module-account`: the module account must hold at least the sum of all active
vault balances plus pending withdrawals in the bond denom. The module account
is also in the bank blocked-address list so users cannot `MsgSend` funds into
it by accident.

## Deliberate stage-1 scope limits

Deliberately out of scope for stage 1, per the design document's timeline:

- Validator-set selection changes (the simple/complex checks) do not touch
  consensus in stage 1; a shadow ranking query ships instead.
- The Reserve Liquidity Fund (RLF), the bubble, and pbluechips are later
  stages and are not touched by any of this code.
