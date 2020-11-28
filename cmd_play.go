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
	"path/filepath"
	"syscall"
	"time"

	"github.com/blugelabs/bluge"
	"github.com/briandowns/spinner"
	"github.com/h2non/filetype"
	"github.com/rubiojr/rapi"
	"github.com/rubiojr/rapi/repository"
	"github.com/rubiojr/rapi/restic"
	"github.com/rubiojr/rindex"
	"github.com/rubiojr/rplay/internal/acoustid"
	"github.com/rubiojr/rplay/internal/fps"
	"github.com/urfave/cli/v2"
)

var repoID = ""

var idx rindex.Indexer
var fetchMetadata = false
var overrideMetadata = false

func init() {
	cmd := &cli.Command{
		Name:   "play",
		Usage:  "Play a song (random if no argument given)",
		Action: playCmd,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "fetch-metadata",
				Required:    false,
				Destination: &fetchMetadata,
			},
			&cli.BoolFlag{
				Name:        "override-metadata",
				Required:    false,
				Destination: &overrideMetadata,
			},
		},
	}
	appCommands = append(appCommands, cmd)

	cmd = &cli.Command{
		Name:   "random",
		Usage:  "Play songs randomly and endlessly",
		Action: playCmd,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "fetch-metadata",
				Required:    false,
				Destination: &fetchMetadata,
			},
			&cli.BoolFlag{
				Name:        "override-metadata",
				Required:    false,
				Destination: &overrideMetadata,
			},
		},
	}
	appCommands = append(appCommands, cmd)
}

func randomize() (string, error) {
	var err error
	hits := []string{}

	count, err := idx.Search("repository_id:"+repoID, func(field string, value []byte) bool {
		if field == "_id" {
			hits = append(hits, string(value))
		}
		return true
	}, nil)

	if count == 0 {
		return "", errors.New("no songs found")
	}

	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(len(hits))

	return hits[r], err
}

func playCmd(c *cli.Context) error {
	initApp()

	// overrideMetadata also means fetchMetadata
	if overrideMetadata {
		fetchMetadata = overrideMetadata
	}

	if fetchMetadata && acoustid.FindFPCALC() == "" {
		fmt.Fprintln(os.Stderr, "\n⚠️  fpcalc not found, acousting fingerprinting won't work\n")
	}

	// Fail fast if index does not exist
	playerReader, err := bluge.OpenReader(blugeConf)
	if err != nil {
		return errNeedsIndex
	}
	playerReader.Close()

	idx, err = rindex.New(indexPath, globalOptions.Repo, globalOptions.Password)
	if err != nil {
		return err
	}

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

	meta := map[string][]byte{}
	var blobBytes []byte
	_, err := idx.Search("_id:"+id, func(field string, value []byte) bool {
		if field == "blobs" {
			blobBytes, _ = fetchBlobs(repo, value)
			return true
		}
		if !filterFieldPlay(field) {
			meta[field] = value
		}
		return true
	}, nil)
	if err != nil {
		return err
	}

	if len(meta) == 0 {
		return fmt.Errorf("no MP3 file found with ID %s", id)
	}

	if blobBytes == nil {
		return fmt.Errorf("error fetching song %s content", id)
	}

	kind, err := filetype.Match(blobBytes)
	if err != nil {
		return err
	}

	if fetchMetadata {
		s.Suffix = " 🌍 fetching metadata..."
		err := fixMetadata(blobBytes, meta)
		if err == nil {
			s.Suffix = " 🌍 Metadata found"
		}
	}

	s.Stop()

	for k, v := range meta {
		printMetadata(k, v, headerColor)
	}

	if kind.MIME.Value == "" {
		return fmt.Errorf("mime type not found. damaged file?")
	}

	switch kind.MIME.Value {
	case "audio/mpeg":
		return play(ctx, "mp3", blobBytes)
	case "audio/ogg":
		return play(ctx, "ogg", blobBytes)
	default:
		return fmt.Errorf("mime type '%s' not supported", kind.MIME.Value)
	}
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

func fixMetadata(song []byte, meta map[string][]byte) error {
	fprinter := fps.New(filepath.Join(defaultIndexDir(), "acoustid.db"))
	breader := bytes.NewReader(song)

	fmeta, err := fprinter.Fingerprint(breader)
	if err != nil {
		return err
	}

	if string(meta["artist"]) == "" || overrideMetadata {
		meta["artist"] = []byte(fmeta.Artist + " 🌍")
	}
	if string(meta["title"]) == "" || overrideMetadata {
		meta["title"] = []byte(fmeta.Title + " 🌍")
	}
	if string(meta["album"]) == "" || overrideMetadata {
		meta["album"] = []byte(fmeta.Album + " 🌍")
	}

	return nil
}
