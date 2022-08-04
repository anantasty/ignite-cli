package chain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ignite/cli/ignite/pkg/cache"
	"github.com/ignite/cli/ignite/pkg/cosmosanalysis/module"
	"github.com/ignite/cli/ignite/pkg/cosmosgen"
)

const (
	defaultVuexPath    = "vue/src/store"
	defaultDartPath    = "flutter/lib"
	defaultOpenAPIPath = "docs/static/openapi.yml"
)

type generateOptions struct {
	isGoEnabled      bool
	isVuexEnabled    bool
	isDartEnabled    bool
	isOpenAPIEnabled bool
}

// GenerateTarget is a target to generate code for from proto files.
type GenerateTarget func(*generateOptions)

// GenerateGo enables generating proto based Go code needed for the chain's source code.
func GenerateGo() GenerateTarget {
	return func(o *generateOptions) {
		o.isGoEnabled = true
	}
}

// GenerateVuex enables generating proto based Vuex store.
func GenerateVuex() GenerateTarget {
	return func(o *generateOptions) {
		o.isVuexEnabled = true
	}
}

// GenerateDart enables generating Dart client.
func GenerateDart() GenerateTarget {
	return func(o *generateOptions) {
		o.isDartEnabled = true
	}
}

// GenerateOpenAPI enables generating OpenAPI spec for your chain.
func GenerateOpenAPI() GenerateTarget {
	return func(o *generateOptions) {
		o.isOpenAPIEnabled = true
	}
}

// generateFromConfig makes code generation from proto files from the given config
func (c *Chain) generateFromConfig(ctx context.Context, cacheStorage cache.Storage, generateGo bool) error {
	conf, err := c.Config()
	if err != nil {
		return err
	}

	var targets []GenerateTarget

	// generate Go pb files
	if generateGo {
		targets = append(targets, GenerateGo())
	}

	// parse config for additional target
	if conf.Client.Vuex.Path != "" {
		targets = append(targets, GenerateVuex())
	}

	if conf.Client.Dart.Path != "" {
		targets = append(targets, GenerateDart())
	}

	if conf.Client.OpenAPI.Path != "" {
		targets = append(targets, GenerateOpenAPI())
	}

	return c.Generate(ctx, cacheStorage, targets...)
}

// Generate makes code generation from proto files for given targets.
func (c *Chain) Generate(
	ctx context.Context,
	cacheStorage cache.Storage,
	targets ...GenerateTarget,
) error {
	// nothing to generate
	if len(targets) == 0 {
		return nil
	}

	var targetOptions generateOptions
	for _, apply := range targets {
		apply(&targetOptions)
	}

	conf, err := c.Config()
	if err != nil {
		return err
	}

	if err := cosmosgen.InstallDependencies(ctx, c.app.Path); err != nil {
		return err
	}

	fmt.Fprintln(c.stdLog().out, "🛠️  Building proto...")

	options := []cosmosgen.Option{
		cosmosgen.IncludeDirs(conf.Build.Proto.ThirdPartyPaths),
	}

	if targetOptions.isGoEnabled {
		options = append(options, cosmosgen.WithGoGeneration(c.app.ImportPath))
	}

	enableThirdPartyModuleCodegen := !c.protoBuiltAtLeastOnce && c.options.isThirdPartyModuleCodegenEnabled

	// generate Vuex code as well if it is enabled.
	if targetOptions.isVuexEnabled {
		vuexPath := conf.Client.Vuex.Path
		if vuexPath == "" {
			vuexPath = defaultVuexPath
		}

		storeRootPath := filepath.Join(c.app.Path, vuexPath, "generated")
		if err := os.MkdirAll(storeRootPath, 0o766); err != nil {
			return err
		}

		options = append(options,
			cosmosgen.WithVuexGeneration(
				enableThirdPartyModuleCodegen,
				cosmosgen.VuexStoreModulePath(storeRootPath),
				storeRootPath,
			),
		)
	}

	if targetOptions.isDartEnabled {
		dartPath := conf.Client.Dart.Path

		if dartPath == "" {
			dartPath = defaultDartPath
		}

		rootPath := filepath.Join(c.app.Path, dartPath, "generated")
		if err := os.MkdirAll(rootPath, 0o766); err != nil {
			return err
		}

		options = append(options,
			cosmosgen.WithDartGeneration(
				enableThirdPartyModuleCodegen,
				func(m module.Module) string {
					return filepath.Join(rootPath, m.Pkg.Name, "module")
				},
				rootPath,
			),
		)
	}

	if targetOptions.isOpenAPIEnabled {
		openAPIPath := conf.Client.OpenAPI.Path

		if openAPIPath == "" {
			openAPIPath = defaultOpenAPIPath
		}

		options = append(options, cosmosgen.WithOpenAPIGeneration(openAPIPath))
	}

	if err := cosmosgen.Generate(ctx, cacheStorage, c.app.Path, conf.Build.Proto.Path, options...); err != nil {
		return &CannotBuildAppError{err}
	}

	c.protoBuiltAtLeastOnce = true

	return nil
}
