package main

import (
	"fmt"
	"log"

	"github.com/rubiojr/rindex/blugeindex"
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
	idx := blugeindex.NewBlugeIndex(c.String("index-path"), 0)

	fmt.Printf("Searching for %s...\n", q)

	reader, err := idx.OpenReader()
	if err != nil {
		return err
	}

	documentMatchIterator, err := idx.SearchWithReaderAndQuery(q, reader)
	if err != nil {
		return err
	}

	count := 0
	match, err := documentMatchIterator.Next()
	for err == nil && match != nil {
		err = match.VisitStoredFields(func(field string, value []byte) bool {
			if filterField(field) && !verbose {
				return true
			}
			printMetadata(field, value, headerColor)
			return true
		})
		if err != nil {
			log.Fatalf("error loading stored fields: %v", err)
		}

		fmt.Println()
		count++
		match, err = documentMatchIterator.Next()
	}

	fmt.Printf("Results: %d\n", count)

	return err
}
