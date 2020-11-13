package main

import (
	"fmt"
	"log"
	"strings"

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

	documentMatchIterator, err := idx.SearchWithReader(q, reader)
	if err != nil {
		return err
	}

	filterField := func(name string) bool {
		switch name {
		case "_id", "album", "genre", "year", "filename", "title", "artist":
			return false
		default:
			return true
		}
	}

	match, err := documentMatchIterator.Next()
	for err == nil && match != nil {
		err = match.VisitStoredFields(func(field string, value []byte) bool {
			f := strings.Title(field)
			if field == "_id" {
				f = "ID"
			}
			if filterField(field) && !verbose {
				return true
			}
			v := string(value)
			if v == "" {
				v = "unknown"
			}
			printRow(f, v, headerColor)
			return true
		})
		if err != nil {
			log.Fatalf("error loading stored fields: %v", err)
		}

		fmt.Println()
		match, err = documentMatchIterator.Next()
	}

	return err
}
