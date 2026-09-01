package app_test

import (
	"testing"

	"cosmossdk.io/log"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client/flags"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/stretchr/testify/require"

	"bluechipChain/app"
)

// TestAppInstantiation builds the full application, exercising the depinject
// wiring (module configs, keeper injection, staking hooks registration). A
// mis-wired module fails here rather than at node startup.
func TestAppInstantiation(t *testing.T) {
	db := dbm.NewMemDB()
	appOptions := make(simtestutil.AppOptionsMap, 0)
	appOptions[flags.FlagHome] = t.TempDir()

	bApp, err := app.New(log.NewNopLogger(), db, nil, true, appOptions)
	require.NoError(t, err)
	require.Equal(t, app.Name, bApp.Name())

	// The liquidityvault keeper must be wired with the governance authority.
	require.NotEmpty(t, bApp.LiquidityvaultKeeper.GetAuthority())
}
