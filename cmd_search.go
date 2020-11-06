package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/blugelabs/bluge"
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
			&cli.BoolFlag{
				Name:     "regexp",
				Aliases:  []string{"r"},
				Usage:    "Query is a regular expression",
				Required: false,
			},
		},
	}
	appCommands = append(appCommands, cmd)
}

func doSearch(c *cli.Context) error {
	initApp()
	return search(c.Args().Get(0), c.Bool("verbose"), c.Bool("regexp"))
}

func search(q string, verbose, regexp bool) error {
	reader, err := bluge.OpenReader(blugeConf)
	if err != nil {
		return errors.New("error opening index")
	}
	defer func() {
		reader.Close()
	}()

	nQuery := strings.ToLower(q)
	var query bluge.Query
	if regexp {
		query = bluge.NewRegexpQuery(nQuery).SetField("filename")
	} else {
		query = bluge.NewWildcardQuery(nQuery).SetField("filename")
	}

	request := bluge.NewAllMatches(query).
		WithStandardAggregations()

	fmt.Printf("Searching for %s...\n", q)

	documentMatchIterator, err := reader.Search(context.Background(), request)
	if err != nil {
		return err
	}

	match, err := documentMatchIterator.Next()
	for err == nil && match != nil {
		err = match.VisitStoredFields(func(field string, value []byte) bool {
			f := strings.Title(field)
			if field == "_id" {
				f = "ID"
			}
			if (field == "tree_id" || field == "blobs" || field == "repository_id" || field == "repository_location") && !verbose {
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
