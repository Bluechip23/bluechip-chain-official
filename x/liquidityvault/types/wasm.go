package types

// Vault Adapter Interface v1.
//
// The JSON messages the liquidityvault module exchanges with registered pool
// contracts. A pool contract must implement this interface to be registrable;
// the mechanics of turning single-sided bond-denom liquidity into a pool
// position (swapping half, CW20 handling, LP NFTs) live entirely on the
// contract side. The module account is always the caller, holds one
// aggregate position per pool at the contract, and tracks per-validator
// ownership through internal shares.
//
// Contract requirements:
//   - provide_liquidity: funds (bond denom only) are attached to the
//     execute; the contract adds them to the caller's position.
//   - withdraw_liquidity{ratio}: removes the given fraction (decimal string
//     in (0, 1]) of the caller's position and returns the proceeds to the
//     caller as bond-denom native funds in the same execution.
//   - position_value{address}: returns the current total value of that
//     address's position, denominated in the bond denom.

// ProvideLiquidityMsg is the execute message adding the attached bond-denom
// funds to the caller's pool position.
type ProvideLiquidityMsg struct {
	ProvideLiquidity struct{} `json:"provide_liquidity"`
}

// WithdrawLiquidityMsg is the execute message removing a fraction of the
// caller's pool position.
type WithdrawLiquidityMsg struct {
	WithdrawLiquidity WithdrawLiquidityPayload `json:"withdraw_liquidity"`
}

// WithdrawLiquidityPayload carries the fraction of the caller's position to
// remove, as a decimal string in (0, 1].
type WithdrawLiquidityPayload struct {
	Ratio string `json:"ratio"`
}

// CollectRewardsMsg is the execute message collecting the accrued liquidity
// fees for the caller's pool position; the contract sends them to the
// caller as bond-denom native funds in the same execution.
type CollectRewardsMsg struct {
	CollectRewards struct{} `json:"collect_rewards"`
}

// PositionValueQuery is the smart query for the current value of an
// address's pool position.
type PositionValueQuery struct {
	PositionValue PositionValuePayload `json:"position_value"`
}

// PositionValuePayload names the position owner being valued.
type PositionValuePayload struct {
	Address string `json:"address"`
}

// PositionValueResponse is the contract's answer to PositionValueQuery: the
// position's total value in the bond denom, as a Uint128 decimal string.
type PositionValueResponse struct {
	Value string `json:"value"`
}
