package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blugelabs/bluge"
	"github.com/briandowns/spinner"
	"github.com/rubiojr/rapi"
	"github.com/rubiojr/rapi/repository"
	"github.com/rubiojr/rapi/restic"
	"github.com/urfave/cli/v2"
)

var repoID = ""
var playerReader *bluge.Reader

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
	documentMatchIterator, err := playerReader.Search(context.Background(), request)
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

	playerReader, err := bluge.OpenReader(blugeConf)
	if err != nil {
		return errNeedsIndex
	}
	defer playerReader.Close()

	err = repo.LoadIndex(context.Background())
	if err != nil {
		return err
	}

	repoID = repo.Config().ID

	id := c.Args().Get(0)
	if id == "" {
		err = randomizeSongs(repo)
	} else {
		fmt.Printf("Playing %s...\n", id)
		err = playSong(context.Background(), id, repo)
	}

	return err
}

func randomizeSongs(repo *repository.Repository) error {
	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan, syscall.SIGINT)
	ctx, cancel := context.WithCancel(context.Background())
	lastCancel := time.Now()
	go func() {
		for {
			s := <-signal_chan
			switch s {
			case syscall.SIGINT:
				now := time.Now()
				if time.Since(lastCancel) < 2*time.Second {
					os.Exit(0)
				}
				lastCancel = now
				cancel()
			default:
			}
		}
	}()

	fmt.Println("Playing a random selection of songs...")
	fmt.Println("Ctrl-C once to play the next song, twice to exit.")
	for {
		id, err := randomize()
		if err != nil {
			return err
		}

		err = playSong(ctx, id, repo)
		switch err {
		case context.Canceled:
			ctx, cancel = context.WithCancel(context.Background())
			continue
		case nil:
			continue
		default:
			return err
		}
	}
}

func playSong(ctx context.Context, id string, repo *repository.Repository) error {
	fmt.Println()
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Color("fgMagenta")
	s.Suffix = " Song found, loading..."

	query := bluge.NewMatchQuery(id).SetField("_id")
	request := bluge.NewAllMatches(query)
	documentMatchIterator, err := playerReader.Search(context.Background(), request)
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
			return true
		}
		if field != "repository_id" && field != "repository_location" && field != "_id" && field != "mod_time" {
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
	return play(ctx, blobBytes)
}

func fetchBlobs(repo *repository.Repository, value []byte) ([]byte, error) {
	var blobs []string

	err := json.Unmarshal(value, &blobs)
	if err != nil {
		return nil, err
	}

	blobBytes := [][]byte{}
	for _, id := range blobs {
		rid, _ := restic.ParseID(id)
		bytes, err := repo.LoadBlob(context.Background(), restic.DataBlob, rid, nil)
		if err != nil {
			return nil, err
		}
		blobBytes = append(blobBytes, bytes)
	}

	return bytes.Join(blobBytes, []byte("")), nil
}
