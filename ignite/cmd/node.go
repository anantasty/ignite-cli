package ignitecmd

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/ignite/cli/ignite/pkg/cosmosclient"
	"github.com/ignite/cli/ignite/pkg/xurl"
	"github.com/spf13/cobra"
)

const (
	flagNode         = "node"
	cosmosRPCAddress = "https://rpc.cosmos.network"
)

func NewNode() *cobra.Command {
	c := &cobra.Command{
		Use:   "node [command]",
		Short: "Make calls to a live blockchain node",
		Args:  cobra.ExactArgs(1),
	}

	c.PersistentFlags().String(flagNode, cosmosRPCAddress, "<host>:<port> to tendermint rpc interface for this chain")

	c.AddCommand(NewNodeQuery())
	c.AddCommand(NewNodeTx())

	return c
}

func newNodeCosmosClient(cmd *cobra.Command) (cosmosclient.Client, error) {
	var (
		home           = getHome(cmd)
		prefix         = getAddressPrefix(cmd)
		node           = getRPC(cmd)
		keyringBackend = getKeyringBackend(cmd)
		keyringDir     = getKeyringDir(cmd)
		gas            = getGas(cmd)
		gasPrices      = getGasPrices(cmd)
		fees           = getFees(cmd)
	)

	options := []cosmosclient.Option{
		cosmosclient.WithAddressPrefix(prefix),
		cosmosclient.WithHome(home),
		cosmosclient.WithKeyringBackend(keyringBackend),
		cosmosclient.WithKeyringDir(keyringDir),
		cosmosclient.WithNodeAddress(xurl.HTTPEnsurePort(node)),
	}

	if gas != "" {
		options = append(options, cosmosclient.WithGas(gas))
	}
	if gasPrices != "" {
		options = append(options, cosmosclient.WithGasPrices(gasPrices))
	}
	if fees != "" {
		options = append(options, cosmosclient.WithFees(fees))
	}

	return cosmosclient.New(cmd.Context(), options...)
}

// lookupAddress returns a bech32 address from an account name or an
// address, or accountNameOrAddress directly if it wasn't found in the keyring
// and if it's a valid bech32 address.
func lookupAddress(client cosmosclient.Client, accountNameOrAddress string) (string, error) {
	a, err := client.Account(accountNameOrAddress)
	if err == nil {
		return a.Info.GetAddress().String(), nil
	}
	// account not found in the keyring, ensure it is a bech32 address
	_, _, err = bech32.DecodeAndConvert(accountNameOrAddress)
	if err != nil {
		return "", fmt.Errorf("'%s' not an account nor a bech32 address: %w", accountNameOrAddress, err)
	}
	return accountNameOrAddress, nil
}

func getRPC(cmd *cobra.Command) (rpc string) {
	rpc, _ = cmd.Flags().GetString(flagNode)
	return
}
