package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/blugelabs/bluge"
	"github.com/rubiojr/rapi"
	"github.com/rubiojr/rapi/repository"
	"github.com/rubiojr/rapi/restic"
	"github.com/urfave/cli/v2"
)

func init() {
	cmd := &cli.Command{
		Name:   "play",
		Usage:  "Play a given song",
		Action: playSong,
	}
	appCommands = append(appCommands, cmd)
}

func randomize() string {
	var found string

	query := bluge.NewWildcardQuery("*").SetField("_id")
	request := bluge.NewAllMatches(query)

	c, _ := blugeReader().Count()
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(int(c))

	documentMatchIterator, err := blugeReader().Search(context.Background(), request)
	counter := int(0)
	match, err := documentMatchIterator.Next()
	for err == nil && match != nil && counter < r {
		err = match.VisitStoredFields(func(field string, value []byte) bool {
			if field == "_id" {
				found = string(value)
			}
			return true
		})

		counter += 1
		match, err = documentMatchIterator.Next()
	}

	return found
}

func playSong(c *cli.Context) error {
	repo, err := rapi.OpenRepository(globalOptions)
	if err != nil {
		return err
	}
	id := c.Args().Get(0)
	if id == "" {
		id = randomize()
	}

	reader, err := bluge.OpenReader(blugeConf)
	if err != nil {
		log.Fatalf("error getting index reader: %v", err)
	}
	defer func() {
		err = reader.Close()
		if err != nil {
			log.Fatalf("error closing reader: %v", err)
		}
	}()

	query := bluge.NewMatchQuery(id).SetField("_id")
	request := bluge.NewAllMatches(query)
	documentMatchIterator, err := reader.Search(context.Background(), request)
	if err != nil {
		log.Fatalf("error executing search: %v", err)
	}

	var blobBytes []byte
	match, err := documentMatchIterator.Next()
	if err != nil {
		return err
	}

	if match == nil {
		return fmt.Errorf("no MP3 file found with ID %s", id)
	}

	meta := map[string]string{}
	var fname string
	err = match.VisitStoredFields(func(field string, value []byte) bool {
		if field == "blobs" {
			blobBytes, err = fetchBlobs(repo, value)
			return true
		}
		if field == "filename" {
			fname = string(value)
		}
		if field != "repository_id" && field != "repository_location" && field != "_id" {
			meta[field] = string(value)
		}
		return true
	})
	if err != nil {
		log.Fatalf("error loading stored fields: %v", err)
	}
	if blobBytes == nil {
		return fmt.Errorf("MP3 '%s' not found in the repository.", fname)
	}

	for k, v := range meta {
		printRow(k, v, headerColor)
	}
	play(blobBytes)
	return nil
}

func fetchBlobs(repo *repository.Repository, value []byte) ([]byte, error) {
	repo.LoadIndex(context.Background())
	var blobBytes []byte
	var blobs []string

	err := json.Unmarshal(value, &blobs)
	if err != nil {
		return nil, err
	}

	for _, id := range blobs {
		rid, _ := restic.ParseID(id)
		bytes, err := repo.LoadBlob(context.Background(), restic.DataBlob, rid, nil)
		if err != nil {
			return nil, err
		}
		blobBytes = append(blobBytes, bytes...)
	}

	return blobBytes, nil
}
