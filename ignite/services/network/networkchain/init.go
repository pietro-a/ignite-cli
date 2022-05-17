package networkchain

import (
	"context"
	"fmt"
	"os"

	"github.com/ignite-hq/cli/ignite/pkg/cache"
	"github.com/ignite-hq/cli/ignite/pkg/cosmosutil/genesis"
	"github.com/ignite-hq/cli/ignite/pkg/events"
)

// Init initializes blockchain by building the binaries and running the init command and
// create the initial genesis of the chain, and set up a validator key
func (c *Chain) Init(ctx context.Context, cacheStorage cache.Storage) (gen *genesis.Genesis, err error) {
	chainHome, err := c.chain.Home()
	if err != nil {
		return nil, err
	}

	// cleanup home dir of app if exists.
	if err = os.RemoveAll(chainHome); err != nil {
		return nil, err
	}

	// build the chain and initialize it with a new validator key
	if _, err := c.Build(ctx, cacheStorage); err != nil {
		return nil, err
	}

	c.ev.Send(events.New(events.StatusOngoing, "Initializing the blockchain"))

	if err = c.chain.Init(ctx, false); err != nil {
		return nil, err
	}

	gen, err = c.initGenesis(ctx)
	if err != nil {
		return nil, err
	}

	c.ev.Send(events.New(events.StatusDone, "Blockchain initialized"))
	c.isInitialized = true
	return gen, nil
}

// initGenesis creates the initial genesis of the genesis depending on the initial genesis type (default, url, ...)
func (c *Chain) initGenesis(ctx context.Context) (gen *genesis.Genesis, err error) {
	c.ev.Send(events.New(events.StatusOngoing, "Computing the Genesis"))

	genesisPath, err := c.chain.GenesisPath()
	if err != nil {
		return nil, err
	}

	// remove existing genesis
	if err := os.RemoveAll(genesisPath); err != nil {
		return nil, err
	}

	// if the blockchain has a genesis URL, the initial genesis is fetched from the URL
	// otherwise, the default genesis is used, which requires no action since the default genesis is generated from the init command
	if c.genesisURL != "" {
		c.ev.Send(events.New(events.StatusOngoing, "Fetching custom Genesis from URL"))
		gen, err = genesis.FromURL(ctx, c.genesisURL, genesisPath)
		if err != nil {
			return nil, err
		}

		if gen.TarballPath() != "" {
			c.ev.Send(
				events.New(events.StatusDone,
					fmt.Sprintf("Extracted custom Genesis from tarball at %s", gen.TarballPath()),
				),
			)
		} else {
			c.ev.Send(events.New(events.StatusDone, "Custom Genesis JSON from URL fetched"))
		}

		hash, err := gen.Hash()
		if err != nil {
			return nil, err
		}

		// if the blockchain has been initialized with no genesis hash, we assign the fetched hash to it
		// otherwise we check the genesis integrity with the existing hash
		if c.genesisHash == "" {
			c.genesisHash = hash
		} else if hash != c.genesisHash {
			return nil, fmt.Errorf("genesis from URL %s is invalid. expected hash %s, actual hash %s", c.genesisURL, c.genesisHash, hash)
		}
	} else {
		// default genesis is used, init CLI command is used to generate it
		cmd, err := c.chain.Commands(ctx)
		if err != nil {
			return nil, err
		}

		// TODO: use validator moniker https://github.com/ignite-hq/cli/issues/1834
		if err := cmd.Init(ctx, "moniker"); err != nil {
			return nil, err
		}

		if gen, err = genesis.FromPath(genesisPath); err != nil {
			return nil, err
		}
	}

	// check the genesis is valid
	if err := c.checkGenesis(ctx); err != nil {
		return nil, err
	}

	c.ev.Send(events.New(events.StatusDone, "Genesis initialized"))
	return gen, nil
}

// checkGenesis checks the stored genesis is valid
func (c *Chain) checkGenesis(ctx context.Context) error {
	// perform static analysis of the chain with the validate-genesis command.
	chainCmd, err := c.chain.Commands(ctx)
	if err != nil {
		return err
	}

	return chainCmd.ValidateGenesis(ctx)

	// TODO: static analysis of the genesis with validate-genesis doesn't check the full validity of the genesis
	// example: gentxs formats are not checked
	// to perform a full validity check of the genesis we must try to start the chain with sample accounts
}
