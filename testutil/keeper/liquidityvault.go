package keeper

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"cosmossdk.io/store"
	"cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"bluechipChain/x/liquidityvault/keeper"
	"bluechipChain/x/liquidityvault/types"
)

// TestBondDenom is the bond denom the mock staking keeper reports.
const TestBondDenom = "ubluechip"

// MockStakingKeeper is a minimal staking keeper for liquidityvault tests.
// Tests control the validator set through the Validators map.
type MockStakingKeeper struct {
	Validators map[string]stakingtypes.Validator
}

func NewMockStakingKeeper() *MockStakingKeeper {
	return &MockStakingKeeper{Validators: make(map[string]stakingtypes.Validator)}
}

// SetValidatorTokens registers (or updates) a validator with the given tokens.
func (m *MockStakingKeeper) SetValidatorTokens(valAddr sdk.ValAddress, tokens math.Int) {
	m.Validators[valAddr.String()] = stakingtypes.Validator{
		OperatorAddress: valAddr.String(),
		Tokens:          tokens,
	}
}

func (m *MockStakingKeeper) GetValidator(_ context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error) {
	validator, found := m.Validators[addr.String()]
	if !found {
		return stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound
	}
	return validator, nil
}

func (m *MockStakingKeeper) GetAllValidators(_ context.Context) ([]stakingtypes.Validator, error) {
	addrs := make([]string, 0, len(m.Validators))
	for addr := range m.Validators {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	validators := make([]stakingtypes.Validator, 0, len(addrs))
	for _, addr := range addrs {
		validators = append(validators, m.Validators[addr])
	}
	return validators, nil
}

func (m *MockStakingKeeper) BondDenom(_ context.Context) (string, error) {
	return TestBondDenom, nil
}

// MockBankKeeper is a minimal bank keeper for liquidityvault tests. It tracks
// balances per address and fails transfers that exceed them.
type MockBankKeeper struct {
	Balances map[string]sdk.Coins
}

func NewMockBankKeeper() *MockBankKeeper {
	return &MockBankKeeper{Balances: make(map[string]sdk.Coins)}
}

func (m *MockBankKeeper) SetBalance(addr sdk.AccAddress, coins sdk.Coins) {
	m.Balances[addr.String()] = coins
}

func (m *MockBankKeeper) GetAllBalances(_ context.Context, addr sdk.AccAddress) sdk.Coins {
	return m.Balances[addr.String()]
}

func (m *MockBankKeeper) SendCoinsFromAccountToModule(_ context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	return m.send(senderAddr, authtypes.NewModuleAddress(recipientModule), amt)
}

func (m *MockBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	return m.send(authtypes.NewModuleAddress(senderModule), recipientAddr, amt)
}

func (m *MockBankKeeper) send(from, to sdk.AccAddress, amt sdk.Coins) error {
	fromBalance := m.Balances[from.String()]
	newFromBalance, negative := fromBalance.SafeSub(amt...)
	if negative {
		return fmt.Errorf("insufficient funds: %s has %s, wants to send %s", from, fromBalance, amt)
	}
	m.Balances[from.String()] = newFromBalance
	m.Balances[to.String()] = m.Balances[to.String()].Add(amt...)
	return nil
}

// LiquidityvaultKeeper builds a liquidityvault keeper over an in-memory store
// with mock bank, staking, and wasm keepers, and default params set.
func LiquidityvaultKeeper(t testing.TB) (keeper.Keeper, sdk.Context, *MockStakingKeeper, *MockBankKeeper, *MockWasmKeeper) {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	require.NoError(t, stateStore.LoadLatestVersion())

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	authority := authtypes.NewModuleAddress(govtypes.ModuleName)

	stakingKeeper := NewMockStakingKeeper()
	bankKeeper := NewMockBankKeeper()

	k := keeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(storeKey),
		log.NewNopLogger(),
		authority.String(),
		bankKeeper,
		stakingKeeper,
	)

	wasmKeeper := NewMockWasmKeeper(bankKeeper)
	k.SetWasmKeeper(wasmKeeper)

	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger())

	require.NoError(t, k.SetParams(ctx, types.DefaultParams()))

	return k, ctx, stakingKeeper, bankKeeper, wasmKeeper
}
