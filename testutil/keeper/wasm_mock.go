package keeper

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// MockPool simulates one pool contract implementing the Vault Adapter
// Interface. Value is the module account's position value; tests can bump it
// to simulate gains and losses.
type MockPool struct {
	ContractAddr sdk.AccAddress
	Value        math.Int
	FailExecute  bool
	FailQuery    bool

	// DepositFeeBps simulates slippage/fees on provide_liquidity: the
	// position value grows by the deposit minus this many basis points.
	DepositFeeBps int64

	// AccruedFees is what the next collect_rewards pays out. Tests fund it
	// via AccrueFees, which also mints the backing balance to the contract.
	AccruedFees math.Int
}

// MockWasmKeeper simulates the wasm bridge for liquidityvault tests. It
// moves real balances in the MockBankKeeper the way wasmd would: deposits
// transfer caller -> contract, withdrawals transfer contract -> caller.
type MockWasmKeeper struct {
	Bank  *MockBankKeeper
	Pools map[string]*MockPool
}

func NewMockWasmKeeper(bank *MockBankKeeper) *MockWasmKeeper {
	return &MockWasmKeeper{Bank: bank, Pools: make(map[string]*MockPool)}
}

// AddPool registers a simulated pool contract at the given address.
func (m *MockWasmKeeper) AddPool(contractAddr sdk.AccAddress) *MockPool {
	pool := &MockPool{ContractAddr: contractAddr, Value: math.ZeroInt(), AccruedFees: math.ZeroInt()}
	m.Pools[contractAddr.String()] = pool
	return pool
}

// AccrueFees simulates the pool earning trading fees for the module's
// position: the amount becomes collectable and the contract receives the
// backing balance.
func (m *MockWasmKeeper) AccrueFees(contractAddr sdk.AccAddress, amount math.Int) {
	pool := m.Pools[contractAddr.String()]
	pool.AccruedFees = pool.AccruedFees.Add(amount)
	m.Bank.Balances[contractAddr.String()] = m.Bank.Balances[contractAddr.String()].Add(sdk.NewCoin(TestBondDenom, amount))
}

// SetPoolValue simulates the pool position gaining or losing value. It keeps
// the contract's bank balance in sync so withdrawals can actually be paid —
// as a real pool holds the assets backing the position value it reports.
func (m *MockWasmKeeper) SetPoolValue(contractAddr sdk.AccAddress, value math.Int) {
	pool := m.Pools[contractAddr.String()]
	pool.Value = value
	m.Bank.SetBalance(contractAddr, sdk.NewCoins(sdk.NewCoin(TestBondDenom, value)))
}

func (m *MockWasmKeeper) Execute(ctx sdk.Context, contractAddress, caller sdk.AccAddress, msg []byte, coins sdk.Coins) ([]byte, error) {
	pool, ok := m.Pools[contractAddress.String()]
	if !ok {
		return nil, fmt.Errorf("no contract at %s", contractAddress)
	}
	if pool.FailExecute {
		return nil, fmt.Errorf("mock contract failure")
	}

	var provide types.ProvideLiquidityMsg
	if err := json.Unmarshal(msg, &provide); err == nil && jsonHasKey(msg, "provide_liquidity") {
		if err := m.Bank.send(caller, contractAddress, coins); err != nil {
			return nil, err
		}
		deposited := coins.AmountOf(TestBondDenom)
		fee := deposited.MulRaw(pool.DepositFeeBps).QuoRaw(10_000)
		pool.Value = pool.Value.Add(deposited.Sub(fee))
		return nil, nil
	}

	if jsonHasKey(msg, "collect_rewards") {
		out := pool.AccruedFees
		pool.AccruedFees = math.ZeroInt()
		if out.IsPositive() {
			coins := sdk.NewCoins(sdk.NewCoin(TestBondDenom, out))
			if err := m.Bank.send(contractAddress, caller, coins); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}

	var withdraw types.WithdrawLiquidityMsg
	if err := json.Unmarshal(msg, &withdraw); err == nil && jsonHasKey(msg, "withdraw_liquidity") {
		ratio, err := math.LegacyNewDecFromStr(withdraw.WithdrawLiquidity.Ratio)
		if err != nil {
			return nil, err
		}
		out := ratio.MulInt(pool.Value).TruncateInt()
		pool.Value = pool.Value.Sub(out)
		if out.IsPositive() {
			coins := sdk.NewCoins(sdk.NewCoin(TestBondDenom, out))
			if err := m.Bank.send(contractAddress, caller, coins); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}

	return nil, fmt.Errorf("mock contract: unknown execute msg %s", msg)
}

func (m *MockWasmKeeper) QuerySmart(_ context.Context, contractAddr sdk.AccAddress, req []byte) ([]byte, error) {
	pool, ok := m.Pools[contractAddr.String()]
	if !ok {
		return nil, fmt.Errorf("no contract at %s", contractAddr)
	}
	if pool.FailQuery {
		return nil, fmt.Errorf("mock query failure")
	}
	if !jsonHasKey(req, "position_value") {
		return nil, fmt.Errorf("mock contract: unknown query %s", req)
	}
	return json.Marshal(types.PositionValueResponse{Value: pool.Value.String()})
}

func (m *MockWasmKeeper) HasContractInfo(_ context.Context, contractAddress sdk.AccAddress) bool {
	_, ok := m.Pools[contractAddress.String()]
	return ok
}

func jsonHasKey(bz []byte, key string) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(bz, &probe); err != nil {
		return false
	}
	_, ok := probe[key]
	return ok
}
