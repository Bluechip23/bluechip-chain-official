package app

import (
	"context"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// RegisterUpgradeHandlers registers upgrade handlers for coordinated chain upgrades.
func (app *App) RegisterUpgradeHandlers() {
	// v1.1.0 upgrade handler placeholder.
	// When a chain upgrade is needed, define the migration logic here and
	// coordinate validators to upgrade their binaries at the specified height.
	app.UpgradeKeeper.SetUpgradeHandler(
		"v1.1.0",
		func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)
}
