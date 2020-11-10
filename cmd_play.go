package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/blugelabs/bluge"
	"github.com/briandowns/spinner"
	"github.com/rubiojr/rapi"
	"github.com/rubiojr/rapi/repository"
	"github.com/rubiojr/rapi/restic"
	"github.com/urfave/cli/v2"
)

var repoID = ""

func init() {
	cmd := &cli.Command{
		Name:   "play",
		Usage:  "Play a song (random if no argument given)",
		Action: playCmd,
	}
	appCommands = append(appCommands, cmd)
	cmd = &cli.Command{
		Name:   "random",
		Usage:  "Play songs randomly and endlessly",
		Action: playCmd,
	}
	appCommands = append(appCommands, cmd)
}

func randomize() (string, error) {
	query := bluge.NewMatchPhraseQuery(repoID).SetField("repository_id")
	request := bluge.NewAllMatches(query)

	hits := []string{}
	documentMatchIterator, err := blugeReader().Search(context.Background(), request)
	if err != nil {
		return "", err
	}

	match, err := documentMatchIterator.Next()
	for err == nil && match != nil {
		err = match.VisitStoredFields(func(field string, value []byte) bool {
			if field == "_id" {
				hits = append(hits, string(value))
			}
			return true
		})
		if err != nil {
			return "", err
		}

		match, err = documentMatchIterator.Next()
	}
	if err != nil {
		return "", err
	}

	if len(hits) == 0 {
		return "", errors.New("no songs found")
	}

	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(len(hits))

	return hits[r], err
}

func playCmd(c *cli.Context) error {
	initApp()
	repo, err := rapi.OpenRepository(globalOptions)
	if err != nil {
		return err
	}

	err = repo.LoadIndex(context.Background())
	if err != nil {
		return err
	}

	repoID = repo.Config().ID

	id := c.Args().Get(0)
	if id == "" {
		fmt.Println("Playing a random selection of songs...")
		for {
			id, err = randomize()
			if err != nil {
				return err
			}

			err = playSong(id, repo)
			if err != nil {
				return err
			}
		}
	} else {
		fmt.Printf("Playing %s...\n", id)
		err = playSong(id, repo)
	}

	return err
}

func playSong(id string, repo *repository.Repository) error {
	fmt.Println()
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Color("fgMagenta")
	s.Suffix = " Song found, loading..."
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
		return fmt.Errorf("MP3 '%s' not found in the repository", fname)
	}
	fmt.Println()
	for k, v := range meta {
		printRow(k, v, headerColor)
	}

	s.Stop()
	return play(blobBytes)
}

func fetchBlobs(repo *repository.Repository, value []byte) ([]byte, error) {
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
