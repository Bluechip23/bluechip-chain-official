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
| `MsgUpdateParams` | governance | Sets the stake cap and withdrawal grace period. |

## Parameters

| Param | Default | Meaning |
|---|---|---|
| `stake_cap` | `0` (disabled) | Maximum bond-denom tokens a validator may have bonded directly to the chain. Governance enables it by setting a positive value. |
| `withdrawal_grace_period` | `72h` | Universal delay between a vault withdrawal request and the release of funds ("Liquidity Providing Change" in the design document). |

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

These arrive with later stages of the LPV rollout, per the design document's
timeline:

- No pool allocation yet: vault funds are held by the module, not yet routed
  to liquidity pools / LP NFTs, so the composite score uses the raw vault
  balance rather than the median of six pseudo-random value posts.
- No reward flow yet: `delegator_reward_share` is stored and validated, and
  starts distributing when pool rewards land (stage 1 scope in the document
  is the parameter itself).
- No validator-set selection changes: the simple/complex checks and the
  Reserve Liquidity Fund (RLF) are later stages.
