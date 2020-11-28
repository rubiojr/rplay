package main

import (
	"fmt"

	"github.com/rubiojr/rindex"
	"github.com/urfave/cli/v2"
)

func init() {
	cmd := &cli.Command{
		Name:   "search",
		Usage:  "Search the index",
		Action: doSearch,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:     "verbose",
				Aliases:  []string{"v"},
				Usage:    "Enable verbose output",
				Required: false,
			},
		},
	}
	appCommands = append(appCommands, cmd)
}

func doSearch(c *cli.Context) error {
	initApp()
	q := c.Args().Get(0)
	verbose := c.Bool("verbose")

	idx, err := rindex.New(indexPath, globalOptions.Repo, globalOptions.Password)
	if err != nil {
		return err
	}

	fmt.Printf("Searching for %s...\n", q)

	count, err := idx.Search(q, func(field string, value []byte) bool {
		if filterField(field) && !verbose {
			return true
		}
		printMetadata(field, value, headerColor)
		return true
	}, func() bool {
		fmt.Println()
		return true
	})

	fmt.Printf("Results: %d\n", count)

	return err
}
