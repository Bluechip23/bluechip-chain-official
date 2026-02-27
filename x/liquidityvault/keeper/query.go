package keeper

import (
	"bluechipChain/x/liquidityvault/types"
)

var _ types.QueryServer = Keeper{}
