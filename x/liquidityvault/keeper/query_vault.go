package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/cosmos-sdk/types/query"

	"bluechipChain/x/liquidityvault/types"
)

func (k Keeper) Vault(goCtx context.Context, req *types.QueryVaultRequest) (*types.QueryVaultResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	vault, found := k.GetVault(goCtx, req.ValidatorAddress)
	if !found {
		return nil, status.Errorf(codes.NotFound, "vault not found for validator %s", req.ValidatorAddress)
	}

	return &types.QueryVaultResponse{Vault: &vault}, nil
}

func (k Keeper) AllVaults(goCtx context.Context, req *types.QueryAllVaultsRequest) (*types.QueryAllVaultsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	vaults := k.GetAllVaults(goCtx)

	pageRes := &query.PageResponse{
		Total: uint64(len(vaults)),
	}

	return &types.QueryAllVaultsResponse{
		Vaults:     vaults,
		Pagination: pageRes,
	}, nil
}
