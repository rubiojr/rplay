package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blugelabs/bluge"
	"github.com/briandowns/spinner"
	"github.com/h2non/filetype"
	"github.com/muesli/reflow/truncate"
	"github.com/rubiojr/rapi"
	"github.com/rubiojr/rapi/repository"
	"github.com/rubiojr/rapi/restic"
	"github.com/rubiojr/rindex/blugeindex"
	"github.com/urfave/cli/v2"
)

var repoID = ""

type songVisitor = func(field string, value []byte) bool

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

func visitSongs(query string, visitor songVisitor) error {
	idx := blugeindex.NewBlugeIndex(indexPath, 0)
	defer idx.Close()

	reader, err := idx.OpenReader()
	if err != nil {
		return err
	}
	defer reader.Close()

	iter, err := idx.SearchWithReader(query, reader)
	if err != nil {
		return nil
	}

	match, err := iter.Next()
	for err == nil && match != nil {
		err = match.VisitStoredFields(visitor)
		if err != nil {
			return err
		}
		match, err = iter.Next()
	}

	return err
}

func randomize() (string, error) {
	hits := []string{}

	err := visitSongs("repository_id:"+repoID, func(field string, value []byte) bool {
		if field == "_id" {
			hits = append(hits, string(value))
		}
		return true
	})

	if len(hits) == 0 {
		return "", errors.New("no songs found")
	}

	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(len(hits))

	return hits[r], err
}

func playCmd(c *cli.Context) error {
	initApp()

	// Fail fast if index does not exist
	playerReader, err := bluge.OpenReader(blugeConf)
	if err != nil {
		return errNeedsIndex
	}
	playerReader.Close()

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

		fmt.Println()
		err = playSong(ctx, id, repo)
		switch err {
		case context.Canceled:
			ctx, cancel = context.WithCancel(context.Background())
			continue
		case nil:
			continue
		default:
			fmt.Printf("🛑 %v.", err)
		}
	}
}

func playSong(ctx context.Context, id string, repo *repository.Repository) error {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Color("fgMagenta")
	s.Suffix = " Song found, loading..."
	defer s.Stop()

	found := false
	var fname string
	meta := map[string]string{}
	var blobBytes []byte
	err := visitSongs("_id:"+id, func(field string, value []byte) bool {
		found = true
		if field == "blobs" {
			blobBytes, _ = fetchBlobs(repo, value)
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
		return err
	}

	if !found {
		return fmt.Errorf("no MP3 file found with ID %s", id)
	}

	if blobBytes == nil {
		return fmt.Errorf("error fetching song %s content", id)
	}

	s.Stop()

	kind, err := filetype.Match(blobBytes)
	if err != nil {
		return err
	}
	if kind.MIME.Value != "audio/mpeg" {
		return fmt.Errorf("damaged song '%s' found", truncate.StringWithTail(fname, 40, "..."))
	}

	for k, v := range meta {
		printRow(k, v, headerColor)
	}

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
