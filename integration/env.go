package envtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ignite-hq/cli/ignite/chainconfig"
	"github.com/ignite-hq/cli/ignite/pkg/availableport"
	"github.com/ignite-hq/cli/ignite/pkg/cmdrunner"
	"github.com/ignite-hq/cli/ignite/pkg/cmdrunner/step"
	"github.com/ignite-hq/cli/ignite/pkg/cosmosfaucet"
	"github.com/ignite-hq/cli/ignite/pkg/gocmd"
	"github.com/ignite-hq/cli/ignite/pkg/httpstatuschecker"
	"github.com/ignite-hq/cli/ignite/pkg/xexec"
	"github.com/ignite-hq/cli/ignite/pkg/xurl"
)

const (
	ServeTimeout = time.Minute * 15
	IgniteApp    = "ignite"
	ConfigYML    = "config.yml"
)

var isCI, _ = strconv.ParseBool(os.Getenv("CI"))

// ConfigUpdateFunc defines a function type to update config file values.
type ConfigUpdateFunc func(*chainconfig.Config) error

// Env provides an isolated testing environment and what's needed to
// make it possible.
type Env struct {
	t   *testing.T
	ctx context.Context
}

// New creates a new testing environment.
func New(t *testing.T) Env {
	ctx, cancel := context.WithCancel(context.Background())
	e := Env{
		t:   t,
		ctx: ctx,
	}
	t.Cleanup(cancel)

	if !xexec.IsCommandAvailable(IgniteApp) {
		t.Fatal("ignite needs to be installed")
	}

	return e
}

// SetCleanup registers a function to be called when the test (or subtest) and all its
// subtests complete.
func (e Env) SetCleanup(f func()) {
	e.t.Cleanup(f)
}

// Ctx returns parent context for the test suite to use for cancelations.
func (e Env) Ctx() context.Context {
	return e.ctx
}

type execOptions struct {
	ctx                    context.Context
	shouldErr, shouldRetry bool
	stdout, stderr         io.Writer
}

type ExecOption func(*execOptions)

// ExecShouldError sets the expectations of a command's execution to end with a failure.
func ExecShouldError() ExecOption {
	return func(o *execOptions) {
		o.shouldErr = true
	}
}

// ExecCtx sets cancelation context for the execution.
func ExecCtx(ctx context.Context) ExecOption {
	return func(o *execOptions) {
		o.ctx = ctx
	}
}

// ExecStdout captures stdout of an execution.
func ExecStdout(w io.Writer) ExecOption {
	return func(o *execOptions) {
		o.stdout = w
	}
}

// ExecStderr captures stderr of an execution.
func ExecStderr(w io.Writer) ExecOption {
	return func(o *execOptions) {
		o.stderr = w
	}
}

// ExecRetry retries command until it is successful before context is canceled.
func ExecRetry() ExecOption {
	return func(o *execOptions) {
		o.shouldRetry = true
	}
}

type clientOptions struct {
	env                        map[string]string
	pattern, rootDir, testfile string
}

// ClientOption defines options for the TS client test runner.
type ClientOption func(*clientOptions)

// ClientEnv option defines environment values for the tests.
func ClientEnv(env map[string]string) ClientOption {
	return func(o *clientOptions) {
		for k, v := range env {
			o.env[k] = v
		}
	}
}

// ClientTestName option defines a pattern to match the test(s) that should be run.
func ClientTestName(pattern string) ClientOption {
	return func(o *clientOptions) {
		o.pattern = pattern
	}
}

// ClientTestDir option defines a root directory where to look for tests and test files.
func ClientTestDir(dir string) ClientOption {
	return func(o *clientOptions) {
		o.rootDir = dir
	}
}

// ClientTestFile option defines a file to look for tests.
func ClientTestFile(filename string) ClientOption {
	return func(o *clientOptions) {
		o.testfile = filename
	}
}

// Exec executes a command step with options where msg describes the expectation from the test.
// unless calling with Must(), Exec() will not exit test runtime on failure.
func (e Env) Exec(msg string, steps step.Steps, options ...ExecOption) (ok bool) {
	opts := &execOptions{
		ctx:    e.ctx,
		stdout: io.Discard,
		stderr: io.Discard,
	}
	for _, o := range options {
		o(opts)
	}
	var (
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)
	copts := []cmdrunner.Option{
		cmdrunner.DefaultStdout(io.MultiWriter(stdout, opts.stdout)),
		cmdrunner.DefaultStderr(io.MultiWriter(stderr, opts.stderr)),
	}
	if isCI {
		copts = append(copts, cmdrunner.EndSignal(os.Kill))
	}
	err := cmdrunner.
		New(copts...).
		Run(opts.ctx, steps...)
	if err == context.Canceled {
		err = nil
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if opts.shouldRetry && opts.ctx.Err() == nil {
			time.Sleep(time.Second)
			return e.Exec(msg, steps, options...)
		}
	}
	if err != nil {
		msg = fmt.Sprintf("%s\n\nLogs:\n\n%s\n\nError Logs:\n\n%s\n",
			msg,
			stdout.String(),
			stderr.String())
	}
	if opts.shouldErr {
		return assert.Error(e.t, err, msg)
	}
	return assert.NoError(e.t, err, msg)
}

const (
	Stargate = "stargate"
)

// Scaffold scaffolds an app to a unique appPath and returns it.
func (e Env) Scaffold(name string, flags ...string) (appPath string) {
	root := e.TmpDir()
	e.Exec("scaffold an app",
		step.NewSteps(step.New(
			step.Exec(
				IgniteApp,
				append([]string{
					"scaffold",
					"chain",
					name,
				}, flags...)...,
			),
			step.Workdir(root),
		)),
	)

	appDir := path.Base(name)

	// Cleanup the home directory of the app
	e.t.Cleanup(func() {
		os.RemoveAll(filepath.Join(e.Home(), fmt.Sprintf(".%s", appDir)))
	})

	return filepath.Join(root, appDir)
}

// Serve serves an application lives under path with options where msg describes the
// execution from the serving action.
// unless calling with Must(), Serve() will not exit test runtime on failure.
func (e Env) Serve(msg, path, home, configPath string, options ...ExecOption) (ok bool) {
	serveCommand := []string{
		"chain",
		"serve",
		"-v",
	}

	if home != "" {
		serveCommand = append(serveCommand, "--home", home)
	}
	if configPath != "" {
		serveCommand = append(serveCommand, "--config", configPath)
	}

	return e.Exec(msg,
		step.NewSteps(step.New(
			step.Exec(IgniteApp, serveCommand...),
			step.Workdir(path),
		)),
		options...,
	)
}

// Simulate runs the simulation test for the app
func (e Env) Simulate(appPath string, numBlocks, blockSize int) {
	e.Exec("running the simulation tests",
		step.NewSteps(step.New(
			step.Exec(
				IgniteApp,
				"chain",
				"simulate",
				"--numBlocks",
				strconv.Itoa(numBlocks),
				"--blockSize",
				strconv.Itoa(blockSize),
			),
			step.Workdir(appPath),
		)),
	)
}

// EnsureAppIsSteady ensures that app living at the path can compile and its tests
// are passing.
func (e Env) EnsureAppIsSteady(appPath string) {
	_, statErr := os.Stat(filepath.Join(appPath, ConfigYML))
	require.False(e.t, os.IsNotExist(statErr), "config.yml cannot be found")

	e.Exec("make sure app is steady",
		step.NewSteps(step.New(
			step.Exec(gocmd.Name(), "test", "./..."),
			step.Workdir(appPath),
		)),
	)
}

// IsAppServed checks that app is served properly and servers are started to listening
// before ctx canceled.
func (e Env) IsAppServed(ctx context.Context, host chainconfig.Host) error {
	checkAlive := func() error {
		addr, err := xurl.HTTP(host.API)
		if err != nil {
			return err
		}

		ok, err := httpstatuschecker.Check(ctx, fmt.Sprintf("%s/node_info", addr))
		if err == nil && !ok {
			err = errors.New("app is not online")
		}
		return err
	}
	return backoff.Retry(checkAlive, backoff.WithContext(backoff.NewConstantBackOff(time.Second), ctx))
}

// IsFaucetServed checks that faucet of the app is served properly
func (e Env) IsFaucetServed(ctx context.Context, faucetClient cosmosfaucet.HTTPClient) error {
	checkAlive := func() error {
		_, err := faucetClient.FaucetInfo(ctx)
		return err
	}
	return backoff.Retry(checkAlive, backoff.WithContext(backoff.NewConstantBackOff(time.Second), ctx))
}

// TmpDir creates a new temporary directory.
func (e Env) TmpDir() (path string) {
	path, err := os.MkdirTemp("", "integration")
	require.NoError(e.t, err, "create a tmp dir")
	e.t.Cleanup(func() { os.RemoveAll(path) })
	return path
}

// RandomizeServerPorts randomizes server ports for the app at path, updates
// its config.yml and returns new values.
func (e Env) RandomizeServerPorts(path string, configFile string) chainconfig.Host {
	if configFile == "" {
		configFile = ConfigYML
	}

	// generate random server ports and servers list.
	ports, err := availableport.Find(6)
	require.NoError(e.t, err)

	genAddr := func(port int) string {
		return fmt.Sprintf("localhost:%d", port)
	}

	servers := chainconfig.Host{
		RPC:     genAddr(ports[0]),
		P2P:     genAddr(ports[1]),
		Prof:    genAddr(ports[2]),
		GRPC:    genAddr(ports[3]),
		GRPCWeb: genAddr(ports[4]),
		API:     genAddr(ports[5]),
	}

	// update config.yml with the generated servers list.
	configyml, err := os.OpenFile(filepath.Join(path, configFile), os.O_RDWR|os.O_CREATE, 0755)
	require.NoError(e.t, err)
	defer configyml.Close()

	var conf chainconfig.Config
	require.NoError(e.t, yaml.NewDecoder(configyml).Decode(&conf))

	conf.Host = servers
	require.NoError(e.t, configyml.Truncate(0))
	_, err = configyml.Seek(0, 0)
	require.NoError(e.t, err)
	require.NoError(e.t, yaml.NewEncoder(configyml).Encode(conf))

	return servers
}

// ConfigureFaucet finds a random port for the app faucet and updates config.yml with this port and provided coins options
func (e Env) ConfigureFaucet(path string, configFile string, coins, coinsMax []string) string {
	if configFile == "" {
		configFile = ConfigYML
	}

	// find a random available port
	port, err := availableport.Find(1)
	require.NoError(e.t, err)

	configyml, err := os.OpenFile(filepath.Join(path, configFile), os.O_RDWR|os.O_CREATE, 0755)
	require.NoError(e.t, err)
	defer configyml.Close()

	var conf chainconfig.Config
	require.NoError(e.t, yaml.NewDecoder(configyml).Decode(&conf))

	conf.Faucet.Port = port[0]
	conf.Faucet.Coins = coins
	conf.Faucet.CoinsMax = coinsMax
	require.NoError(e.t, configyml.Truncate(0))
	_, err = configyml.Seek(0, 0)
	require.NoError(e.t, err)
	require.NoError(e.t, yaml.NewEncoder(configyml).Encode(conf))

	addr, err := xurl.HTTP(fmt.Sprintf("0.0.0.0:%d", port[0]))
	require.NoError(e.t, err)

	return addr
}

// UpdateConfig updates config.yml file values.
func (e Env) UpdateConfig(path, configFile string, update ConfigUpdateFunc) {
	if configFile == "" {
		configFile = ConfigYML
	}

	f, err := os.OpenFile(filepath.Join(path, configFile), os.O_RDWR|os.O_CREATE, 0755)
	require.NoError(e.t, err)

	defer f.Close()

	var cfg chainconfig.Config

	require.NoError(e.t, yaml.NewDecoder(f).Decode(&cfg))
	require.NoError(e.t, update(&cfg))
	require.NoError(e.t, f.Truncate(0))

	_, err = f.Seek(0, 0)

	require.NoError(e.t, err)
	require.NoError(e.t, yaml.NewEncoder(f).Encode(cfg))
}

// SetRandomHomeConfig sets in the blockchain config files generated temporary directories for home directories
func (e Env) SetRandomHomeConfig(path string, configFile string) {
	if configFile == "" {
		configFile = ConfigYML
	}

	// update config.yml with the generated temporary directories
	configyml, err := os.OpenFile(filepath.Join(path, configFile), os.O_RDWR|os.O_CREATE, 0755)
	require.NoError(e.t, err)
	defer configyml.Close()

	var conf chainconfig.Config
	require.NoError(e.t, yaml.NewDecoder(configyml).Decode(&conf))

	conf.Init.Home = e.TmpDir()
	require.NoError(e.t, configyml.Truncate(0))
	_, err = configyml.Seek(0, 0)
	require.NoError(e.t, err)
	require.NoError(e.t, yaml.NewEncoder(configyml).Encode(conf))
}

// RunClientTests runs the Typescript client tests.
func (e Env) RunClientTests(path string, options ...ClientOption) bool {
	npm, err := exec.LookPath("npm")
	require.NoError(e.t, err, "npm binary not found")

	cwd, err := os.Getwd()
	require.NoError(e.t, err)

	// The filename of this module is required to be able to define the location
	// of the TS client test runner package to be used as working directory when
	// running the tests.
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		e.t.Fatal("failed to read file name")
	}

	opts := clientOptions{
		rootDir: "",
		env: map[string]string{
			"TEST_CHAIN_PATH": path,
		},
	}
	for _, o := range options {
		o(&opts)
	}

	var (
		output bytes.Buffer
		env    []string
	)

	//  Install the dependencies needed to run TS client tests
	ok = e.Exec("install client dependencies", step.NewSteps(
		step.New(
			step.Workdir(fmt.Sprintf("%s/vue", path)),
			step.Stdout(&output),
			step.Exec(npm, "install"),
			step.PostExec(func(err error) error {
				// Print the npm output when there is an error
				if err != nil {
					e.t.Log("\n", output.String())
				}

				return err
			}),
		),
	))
	if !ok {
		return false
	}

	output.Reset()

	// The root dir for the tests must be an absolute path
	absRootDir := filepath.Join(cwd, opts.rootDir)

	args := []string{"run", "test", "--", "--dir", absRootDir}
	if opts.pattern != "" {
		args = append(args, "-t", opts.pattern)
	}

	if opts.testfile != "" {
		args = append(args, opts.testfile)
	}

	for k, v := range opts.env {
		env = append(env, cmdrunner.Env(k, v))
	}

	// The tests are run from the TS client test runner package directory
	runnerDir := filepath.Join(filepath.Dir(filename), "testdata/tstestrunner")

	// TODO: Ignore stderr ? Errors are already displayed with traceback in the stdout
	return e.Exec("run client tests", step.NewSteps(
		// Make sure the test runner dependencies are installed
		step.New(
			step.Workdir(runnerDir),
			step.Stdout(&output),
			step.Exec(npm, "install"),
			step.PostExec(func(err error) error {
				// Print the npm output when there is an error
				if err != nil {
					e.t.Log("\n", output.String())
				}

				return err
			}),
		),
		// Run the TS client tests
		step.New(
			step.Workdir(runnerDir),
			step.Stdout(&output),
			step.Env(env...),
			step.PreExec(func() error {
				// Clear the output from the previous step
				output.Reset()

				return nil
			}),
			step.Exec(npm, args...),
			step.PostExec(func(err error) error {
				// Always print tests output to be available on errors or when verbose is enabled
				e.t.Log("\n", output.String())

				return err
			}),
		),
	))
}

// Must fails the immediately if not ok.
// t.Fail() needs to be called for the failing tests before running Must().
func (e Env) Must(ok bool) {
	if !ok {
		e.t.FailNow()
	}
}

// Home returns user's home dir.
func (e Env) Home() string {
	home, err := os.UserHomeDir()
	require.NoError(e.t, err)
	return home
}

// AppdHome returns appd's home dir.
func (e Env) AppdHome(name string) string {
	return filepath.Join(e.Home(), fmt.Sprintf(".%s", name))
}
