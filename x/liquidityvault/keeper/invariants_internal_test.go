package keeper

import (
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
	"github.com/stretchr/testify/require"

	"bluechipChain/x/liquidityvault/types"
)

// bareKeeper builds a keeper without the testutil mocks (which would be an
// import cycle from this white-box package). Only store-backed methods may
// be exercised.
func bareKeeper(t *testing.T) (Keeper, sdk.Context) {
	t.Helper()
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	require.NoError(t, stateStore.LoadLatestVersion())

	cdc := codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	k := NewKeeper(
		cdc,
		runtime.NewKVStoreService(storeKey),
		log.NewNopLogger(),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		nil,
		nil,
	)
	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger())
	return k, ctx
}

func TestPoolSharesInvariantDetectsDanglingReservation(t *testing.T) {
	k, ctx := bareKeeper(t)
	valAddr := sdk.ValAddress([]byte("validator-operator-1"))

	// A reservation with no queue entries behind it must break the
	// invariant: it would permanently block the validator's deallocations.
	require.NoError(t, k.setPendingShares(ctx, valAddr, 1, math.NewInt(50)))

	msg, broken := PoolSharesInvariant(k)(ctx)
	require.True(t, broken, msg)
}

func TestPoolSharesInvariantDetectsTotalMismatch(t *testing.T) {
	k, ctx := bareKeeper(t)
	valAddr := sdk.ValAddress([]byte("validator-operator-1"))
	contract := sdk.AccAddress([]byte("pool-contract-1-----"))

	require.NoError(t, k.SetPool(ctx, types.RegisteredPool{
		PoolId:          1,
		ContractAddress: contract.String(),
		Enabled:         true,
	}))
	require.NoError(t, k.setPosition(ctx, types.PoolPosition{
		ValidatorAddress: valAddr.String(),
		PoolId:           1,
		Shares:           math.NewInt(100),
	}))

	// Total-shares record out of sync with the positions.
	require.NoError(t, k.setPoolTotalShares(ctx, 1, math.NewInt(150)))
	msg, broken := PoolSharesInvariant(k)(ctx)
	require.True(t, broken, msg)

	// In sync: invariant holds.
	require.NoError(t, k.setPoolTotalShares(ctx, 1, math.NewInt(100)))
	_, broken = PoolSharesInvariant(k)(ctx)
	require.False(t, broken)
}
