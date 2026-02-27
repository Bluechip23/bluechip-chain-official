package types

// WasmDepositLiquidityMsg is the JSON message sent to the pool contract to deposit liquidity.
type WasmDepositLiquidityMsg struct {
	DepositLiquidity *DepositLiquidityInner `json:"deposit_liquidity"`
}

type DepositLiquidityInner struct {
	Amount0 string `json:"amount0"`
	Amount1 string `json:"amount1"`
}

// WasmCw20SendMsg is the CW20 Send message to deposit creator tokens to the pool.
type WasmCw20SendMsg struct {
	Send *Cw20SendInner `json:"send"`
}

type Cw20SendInner struct {
	Contract string `json:"contract"`
	Amount   string `json:"amount"`
	Msg      string `json:"msg"` // base64-encoded inner message
}

// WasmCw20DepositLiquidityHook is the inner hook message for CW20 deposit.
type WasmCw20DepositLiquidityHook struct {
	DepositLiquidity *Cw20DepositLiquidityInner `json:"DepositLiquidity"`
}

type Cw20DepositLiquidityInner struct {
	Amount0    string `json:"amount0"`
	MinAmount0 string `json:"min_amount0,omitempty"`
	MinAmount1 string `json:"min_amount1,omitempty"`
}

// WasmQueryPositionMsg queries a specific LP position.
type WasmQueryPositionMsg struct {
	Position *QueryPositionInner `json:"position"`
}

type QueryPositionInner struct {
	PositionId string `json:"position_id"`
}

// WasmQueryPositionsByOwnerMsg queries positions owned by a specific address.
type WasmQueryPositionsByOwnerMsg struct {
	PositionsByOwner *QueryPositionsByOwnerInner `json:"positions_by_owner"`
}

type QueryPositionsByOwnerInner struct {
	Owner      string  `json:"owner"`
	StartAfter *string `json:"start_after,omitempty"`
	Limit      *uint32 `json:"limit,omitempty"`
}

// WasmQueryPoolStateMsg queries the pool state.
type WasmQueryPoolStateMsg struct {
	PoolState *struct{} `json:"pool_state"`
}

// WasmPositionResponse is the response from a position query.
type WasmPositionResponse struct {
	PositionId  string `json:"position_id"`
	Liquidity   string `json:"liquidity"`
	Owner       string `json:"owner"`
	UnclaimedFees0 string `json:"unclaimed_fees_0"`
	UnclaimedFees1 string `json:"unclaimed_fees_1"`
}

// WasmPositionsResponse is the response from a positions query.
type WasmPositionsResponse struct {
	Positions []WasmPositionResponse `json:"positions"`
}

// WasmPoolStateResponse is the response from a pool state query.
type WasmPoolStateResponse struct {
	Reserve0       string `json:"reserve0"`
	Reserve1       string `json:"reserve1"`
	TotalLiquidity string `json:"total_liquidity"`
}

// WasmWithdrawLiquidityMsg is the JSON message sent to the pool contract to withdraw liquidity.
type WasmWithdrawLiquidityMsg struct {
	WithdrawLiquidity *WithdrawLiquidityInner `json:"withdraw_liquidity"`
}

type WithdrawLiquidityInner struct {
	PositionId string `json:"position_id"`
}
