package keeper

import (
	"context"
	"fmt"
    
    "cosmossdk.io/math"
	"cosmossdk.io/core/store"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
    authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"bluechipChain/x/fixedmint/types"
)

// ...

// MintFixedBlockReward mints the fixed amount of tokens and sends them to the fee collector.
func (k Keeper) MintFixedBlockReward(ctx context.Context) error {
    amount := sdk.NewCoins(sdk.NewCoin("ubluechip", math.NewInt(1000000)))

    // Mint coins to the fixedmint module account
    if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, amount); err != nil {
        return err
    }

    // Send the minted coins to the fee_collector module account
    if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, authtypes.FeeCollectorName, amount); err != nil {
        return err
    }
    
    k.Logger().Info("Minted fixed block reward", "amount", amount)
    return nil
}

type (
	Keeper struct {
		cdc          codec.BinaryCodec
		storeService store.KVStoreService
		logger       log.Logger

        // the address capable of executing a MsgUpdateParams message. Typically, this
        // should be the x/gov module account.
        authority string
        
		
        bankKeeper types.BankKeeper
	}
)

func NewKeeper(
    cdc codec.BinaryCodec,
	storeService store.KVStoreService,
    logger log.Logger,
	authority string,
    
    bankKeeper types.BankKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address: %s", authority))
	}

	return Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		logger:       logger,
		
		bankKeeper: bankKeeper,
	}
}

// GetAuthority returns the module's authority.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// Logger returns a module-specific logger.
func (k Keeper) Logger() log.Logger {
	return k.logger.With("module", fmt.Sprintf("x/%s", types.ModuleName))
}


