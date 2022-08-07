package node_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ignite/cli/ignite/chainconfig"
	"github.com/ignite/cli/ignite/pkg/cliui/entrywriter"
	"github.com/ignite/cli/ignite/pkg/cmdrunner/step"
	"github.com/ignite/cli/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/ignite/pkg/randstr"
	"github.com/ignite/cli/ignite/pkg/xurl"
	envtest "github.com/ignite/cli/integration"
)

const keyringTestDirName = "keyring-test"
const testPrefix = "testpref"

func TestNodeQueryBankBalances(t *testing.T) {
	var (
		appname = randstr.Runes(10)
		alice   = "alice"

		env     = envtest.New(t)
		app     = env.Scaffold(appname, "--address-prefix", testPrefix)
		home    = env.AppHome(appname)
		servers = app.RandomizeServerPorts()

		accKeyringDir = t.TempDir()
	)

	node, err := xurl.HTTP(servers.RPC)
	require.NoError(t, err)

	ca, err := cosmosaccount.New(
		cosmosaccount.WithHome(filepath.Join(home, keyringTestDirName)),
		cosmosaccount.WithKeyringBackend(cosmosaccount.KeyringMemory),
	)
	require.NoError(t, err)

	aliceAccount, aliceMnemonic, err := ca.Create(alice)
	require.NoError(t, err)

	app.EditConfig(func(conf *chainconfig.Config) {
		conf.Accounts = []chainconfig.Account{
			{
				Name:     alice,
				Mnemonic: aliceMnemonic,
				Coins:    []string{"5600atoken", "1200btoken", "100000000stake"},
			},
		}
		conf.Faucet = chainconfig.Faucet{}
		conf.Init.KeyringBackend = keyring.BackendTest
	})

	env.Must(env.Exec("import alice",
		step.NewSteps(step.New(
			step.Exec(
				envtest.IgniteApp,
				"account",
				"import",
				alice,
				"--keyring-dir", accKeyringDir,
				"--non-interactive",
				"--secret", aliceMnemonic,
			),
		)),
	))

	var (
		ctx, cancel       = context.WithTimeout(env.Ctx(), envtest.ServeTimeout)
		isBackendAliveErr error
	)

	// do not fail the test in a goroutine, it has to be done in the main.
	go func() {
		defer cancel()

		if isBackendAliveErr = env.IsAppServed(ctx, servers); isBackendAliveErr != nil {
			return
		}

		// error "account doesn't have any balances" occurs if a sleep is not included
		// TODO find another way without sleep, with retry+ctx routine.
		time.Sleep(time.Second * 1)

		b := &bytes.Buffer{}

		env.Exec("query bank balances by account name",
			step.NewSteps(step.New(
				step.Exec(
					envtest.IgniteApp,
					"node",
					"query",
					"bank",
					"balances",
					"alice",
					"--node", node,
					"--keyring-dir", accKeyringDir,
					"--address-prefix", testPrefix,
				),
			)),
			envtest.ExecStdout(b),
		)

		if env.HasFailed() {
			return
		}

		var expectedBalances strings.Builder
		entrywriter.MustWrite(&expectedBalances, []string{"Amount", "Denom"},
			[]string{"5600", "atoken"},
			[]string{"1200", "btoken"},
		)
		assert.Contains(t, b.String(), expectedBalances.String())

		b.Reset()
		env.Exec("query bank balances by address",
			step.NewSteps(step.New(
				step.Exec(
					envtest.IgniteApp,
					"node",
					"query",
					"bank",
					"balances",
					aliceAccount.Address(testPrefix),
					"--node", node,
					"--keyring-dir", accKeyringDir,
					"--address-prefix", testPrefix,
				),
			)),
			envtest.ExecStdout(b),
		)

		if env.HasFailed() {
			return
		}

		assert.Contains(t, b.String(), expectedBalances.String())

		b.Reset()
		env.Exec("query bank balances with pagination -page 1",
			step.NewSteps(step.New(
				step.Exec(
					envtest.IgniteApp,
					"node",
					"query",
					"bank",
					"balances",
					"alice",
					"--node", node,
					"--keyring-dir", accKeyringDir,
					"--address-prefix", testPrefix,
					"--limit", "1",
					"--page", "1",
				),
			)),
			envtest.ExecStdout(b),
		)

		if env.HasFailed() {
			return
		}

		expectedBalances.Reset()
		entrywriter.MustWrite(&expectedBalances, []string{"Amount", "Denom"},
			[]string{"5600", "atoken"},
		)
		assert.Contains(t, b.String(), expectedBalances.String())
		assert.NotContains(t, b.String(), "btoken")

		b.Reset()
		env.Exec("query bank balances with pagination -page 2",
			step.NewSteps(step.New(
				step.Exec(
					envtest.IgniteApp,
					"node",
					"query",
					"bank",
					"balances",
					"alice",
					"--node", node,
					"--keyring-dir", accKeyringDir,
					"--address-prefix", testPrefix,
					"--limit", "1",
					"--page", "2",
				),
			)),
			envtest.ExecStdout(b),
		)

		if env.HasFailed() {
			return
		}

		expectedBalances.Reset()
		entrywriter.MustWrite(&expectedBalances, []string{"Amount", "Denom"},
			[]string{"1200", "btoken"},
		)
		assert.Contains(t, b.String(), expectedBalances.String())
		assert.NotContains(t, b.String(), "atoken")

		b.Reset()
		env.Exec("query bank balances fail with non-existent account name",
			step.NewSteps(step.New(
				step.Exec(
					envtest.IgniteApp,
					"node",
					"query",
					"bank",
					"balances",
					"nonexistentaccount",
					"--node", node,
					"--keyring-dir", accKeyringDir,
					"--address-prefix", testPrefix,
				),
			)),
			envtest.ExecShouldError(),
		)

		if env.HasFailed() {
			return
		}

		env.Exec("query bank balances fail with non-existent address",
			step.NewSteps(step.New(
				step.Exec(
					envtest.IgniteApp,
					"node",
					"query",
					"bank",
					"balances",
					testPrefix+"1gspvt8qsk8cryrsxnqt452cjczjm5ejdgla24e",
					"--node", node,
					"--keyring-dir", accKeyringDir,
					"--address-prefix", testPrefix,
				),
			)),
			envtest.ExecShouldError(),
		)

		if env.HasFailed() {
			return
		}

		env.Exec("query bank balances should fail with a wrong prefix",
			step.NewSteps(step.New(
				step.Exec(
					envtest.IgniteApp,
					"node",
					"query",
					"bank",
					"balances",
					"alice",
					"--node", node,
					"--keyring-dir", accKeyringDir,
					// the default prefix will fail this test, which is on purpose.
				),
			)),
			envtest.ExecShouldError(),
		)
	}()

	env.Must(app.Serve("should serve with Stargate version", envtest.ExecCtx(ctx)))

	require.NoError(t, isBackendAliveErr, "app cannot get online in time")
}
