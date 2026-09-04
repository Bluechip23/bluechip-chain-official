package app

import (
	"context"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	liquidityvaulttypes "bluechipChain/x/liquidityvault/types"
)

// UpgradeNameLPVStage1 is the coordinated upgrade introducing the
// liquidityvault module (LPV stage 1: vaults, composite score, stake cap,
// pool allocation, value posts, reward flow).
const UpgradeNameLPVStage1 = "lpv-stage-1"

// RegisterUpgradeHandlers registers upgrade handlers for coordinated chain upgrades.
func (app *App) RegisterUpgradeHandlers() {
	// LPV stage 1: RunMigrations sees the liquidityvault module missing
	// from the version map and runs its InitGenesis (default genesis: cap
	// disabled, 72h grace periods, 20h value-post interval). Governance
	// enables the stake cap afterwards via MsgUpdateParams and registers
	// pools via MsgRegisterPool.
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameLPVStage1,
		func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)

	// The new module's store must be added at the upgrade height, or the
	// node fails to restart into the new binary.
	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(err)
	}
	if upgradeInfo.Name == UpgradeNameLPVStage1 && !app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, &storetypes.StoreUpgrades{
			Added: []string{liquidityvaulttypes.StoreKey},
		}))
	}
}
