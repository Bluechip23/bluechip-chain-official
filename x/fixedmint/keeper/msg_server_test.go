package keeper_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

    keepertest "bluechipChain/testutil/keeper"
    "bluechipChain/x/fixedmint/types"
    "bluechipChain/x/fixedmint/keeper"
)

func setupMsgServer(t testing.TB) (keeper.Keeper, types.MsgServer, context.Context) {
	k, ctx := keepertest.FixedmintKeeper(t)
	return k, keeper.NewMsgServerImpl(k), ctx
}

func TestMsgServer(t *testing.T) {
	k, ms, ctx := setupMsgServer(t)
	require.NotNil(t, ms)
	require.NotNil(t, ctx)
	require.NotEmpty(t, k)
}