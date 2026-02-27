package keeper

import (
	"context"
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

// MockBankKeeper implements types.BankKeeper for testing
type MockBankKeeper struct {
	Balances    map[string]sdk.Coins
	SentCoins   []SentCoinsRecord
	FailOnSend  bool
}

type SentCoinsRecord struct {
	From   string
	To     string
	Amount sdk.Coins
}

func NewMockBankKeeper() *MockBankKeeper {
	return &MockBankKeeper{
		Balances: make(map[string]sdk.Coins),
	}
}

func (m *MockBankKeeper) SpendableCoins(_ context.Context, addr sdk.AccAddress) sdk.Coins {
	return m.Balances[addr.String()]
}

func (m *MockBankKeeper) SendCoins(_ context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error {
	if m.FailOnSend {
		return types.ErrInvalidDepositAmount
	}
	m.SentCoins = append(m.SentCoins, SentCoinsRecord{From: fromAddr.String(), To: toAddr.String(), Amount: amt})
	return nil
}

func (m *MockBankKeeper) SendCoinsFromAccountToModule(_ context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	if m.FailOnSend {
		return types.ErrInvalidDepositAmount
	}
	m.SentCoins = append(m.SentCoins, SentCoinsRecord{From: senderAddr.String(), To: recipientModule, Amount: amt})
	return nil
}

func (m *MockBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	if m.FailOnSend {
		return types.ErrInvalidDepositAmount
	}
	return nil
}

func (m *MockBankKeeper) SendCoinsFromModuleToModule(_ context.Context, senderModule, recipientModule string, amt sdk.Coins) error {
	return nil
}

func (m *MockBankKeeper) MintCoins(_ context.Context, moduleName string, amt sdk.Coins) error {
	return nil
}

// MockStakingKeeper implements types.StakingKeeper for testing
type MockStakingKeeper struct {
	Validators  map[string]stakingtypes.Validator
	Delegations map[string][]stakingtypes.Delegation
	TotalPower  math.Int
}

func NewMockStakingKeeper() *MockStakingKeeper {
	return &MockStakingKeeper{
		Validators:  make(map[string]stakingtypes.Validator),
		Delegations: make(map[string][]stakingtypes.Delegation),
		TotalPower:  math.NewInt(1000000),
	}
}

func (m *MockStakingKeeper) GetValidator(_ context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error) {
	v, ok := m.Validators[addr.String()]
	if !ok {
		return stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound
	}
	return v, nil
}

func (m *MockStakingKeeper) GetAllValidators(_ context.Context) ([]stakingtypes.Validator, error) {
	vals := make([]stakingtypes.Validator, 0, len(m.Validators))
	for _, v := range m.Validators {
		vals = append(vals, v)
	}
	return vals, nil
}

func (m *MockStakingKeeper) GetValidatorDelegations(_ context.Context, valAddr sdk.ValAddress) ([]stakingtypes.Delegation, error) {
	return m.Delegations[valAddr.String()], nil
}

func (m *MockStakingKeeper) GetLastTotalPower(_ context.Context) (math.Int, error) {
	return m.TotalPower, nil
}

// AddValidator adds a validator to the mock with the given bonded tokens
func (m *MockStakingKeeper) AddValidator(valAddr sdk.ValAddress, bondedTokens math.Int) {
	v := stakingtypes.Validator{
		OperatorAddress: valAddr.String(),
		Tokens:          bondedTokens,
		Status:          stakingtypes.Bonded,
	}
	m.Validators[valAddr.String()] = v
}

// MockAccountKeeper implements types.AccountKeeper for testing
type MockAccountKeeper struct {
	ModuleAddresses map[string]sdk.AccAddress
}

func NewMockAccountKeeper() *MockAccountKeeper {
	return &MockAccountKeeper{
		ModuleAddresses: map[string]sdk.AccAddress{
			types.ModuleName: authtypes.NewModuleAddress(types.ModuleName),
		},
	}
}

func (m *MockAccountKeeper) GetAccount(_ context.Context, addr sdk.AccAddress) sdk.AccountI {
	return nil
}

func (m *MockAccountKeeper) GetModuleAddress(moduleName string) sdk.AccAddress {
	addr, ok := m.ModuleAddresses[moduleName]
	if !ok {
		return authtypes.NewModuleAddress(moduleName)
	}
	return addr
}

// MockWasmKeeper implements types.WasmKeeper for testing
type MockWasmKeeper struct {
	ExecuteResults  [][]byte
	ExecuteErrors   []error
	QueryResults    map[string][]byte
	QueryErrors     map[string]error
	ExecuteCalls    int
	QueryCalls      int
}

func NewMockWasmKeeper() *MockWasmKeeper {
	return &MockWasmKeeper{
		QueryResults: make(map[string][]byte),
		QueryErrors:  make(map[string]error),
	}
}

func (m *MockWasmKeeper) Execute(_ sdk.Context, _ sdk.AccAddress, _ sdk.AccAddress, _ []byte, _ sdk.Coins) ([]byte, error) {
	idx := m.ExecuteCalls
	m.ExecuteCalls++
	if idx < len(m.ExecuteErrors) && m.ExecuteErrors[idx] != nil {
		return nil, m.ExecuteErrors[idx]
	}
	if idx < len(m.ExecuteResults) {
		return m.ExecuteResults[idx], nil
	}
	return []byte(`{}`), nil
}

func (m *MockWasmKeeper) QuerySmart(_ context.Context, contractAddr sdk.AccAddress, queryMsg []byte) ([]byte, error) {
	m.QueryCalls++
	key := contractAddr.String()
	if err, ok := m.QueryErrors[key]; ok && err != nil {
		return nil, err
	}
	if res, ok := m.QueryResults[key]; ok {
		return res, nil
	}
	return []byte(`{}`), nil
}

// LiquidityvaultKeeper creates a test keeper with mock dependencies
func LiquidityvaultKeeper(t testing.TB) (keeper.Keeper, sdk.Context, *MockBankKeeper, *MockStakingKeeper, *MockAccountKeeper) {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	require.NoError(t, stateStore.LoadLatestVersion())

	registry := codectypes.NewInterfaceRegistry()
	// Register message implementations directly instead of calling
	// types.RegisterInterfaces which also calls RegisterMsgServiceDesc.
	// The latter panics in tests because the proto file descriptor is not
	// available in the generated tx.pb.go (it was generated without the
	// gzipped file descriptor). Registering just the implementations is
	// sufficient for serialization in tests.
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&types.MsgUpdateParams{},
		&types.MsgRegisterValidator{},
		&types.MsgDepositToVault{},
		&types.MsgSetDelegatorRewardPercent{},
	)
	cdc := codec.NewProtoCodec(registry)
	authority := authtypes.NewModuleAddress(govtypes.ModuleName)

	bankKeeper := NewMockBankKeeper()
	stakingKeeper := NewMockStakingKeeper()
	accountKeeper := NewMockAccountKeeper()

	k := keeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(storeKey),
		log.NewNopLogger(),
		authority.String(),
		bankKeeper,
		stakingKeeper,
		accountKeeper,
	)

	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger())

	// Initialize params
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		panic(err)
	}

	return k, ctx, bankKeeper, stakingKeeper, accountKeeper
}

// LiquidityvaultKeeperWithWasm creates a test keeper with mock wasm keeper
func LiquidityvaultKeeperWithWasm(t testing.TB) (keeper.Keeper, sdk.Context, *MockBankKeeper, *MockStakingKeeper, *MockAccountKeeper, *MockWasmKeeper) {
	k, ctx, bankKeeper, stakingKeeper, accountKeeper := LiquidityvaultKeeper(t)
	wasmKeeper := NewMockWasmKeeper()
	k.SetWasmKeeper(wasmKeeper)
	return k, ctx, bankKeeper, stakingKeeper, accountKeeper, wasmKeeper
}
