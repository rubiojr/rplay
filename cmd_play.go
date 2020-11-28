package main

import (
	"context"
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
	"github.com/muesli/reflow/truncate"
	"github.com/rubiojr/rapi"
	"github.com/rubiojr/rapi/repository"
	"github.com/rubiojr/rindex"
	"github.com/rubiojr/rplay/internal/acoustid"
	"github.com/rubiojr/rplay/internal/fps"
	"github.com/urfave/cli/v2"
)

var repoID = ""

var idx rindex.Indexer
var fetchMetadata = false
var overrideMetadata = false
var tmpFileName string

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
	tmpFileName = filepath.Join(defaultCacheDir(), fmt.Sprintf("song-%d", time.Now().UnixNano()))

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
				cancel()
				now := time.Now()
				if time.Since(lastCancel) < 2*time.Second {
					os.Remove(tmpFileName)
					os.Exit(0)
				}
				lastCancel = now
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
			fmt.Printf("\n\n🛑 %v\n", err)
		}
	}
}

func playSong(ctx context.Context, id string, repo *repository.Repository) error {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Color("fgMagenta")
	s.Suffix = " Song found, buffering..."

	var ssize float64
	meta := map[string][]byte{}
	_, err := idx.Search("_id:"+id, func(field string, value []byte) bool {
		if field == "size" {
			ssize, _ = bluge.DecodeNumericFloat64(value)
			return true
		}
		if !filterFieldPlay(field) {
			meta[field] = value
		}
		return true
	}, nil)

	title := string(meta["title"])
	if title != "" {
		s.Suffix = fmt.Sprintf(" Song found, buffering '%s'...", truncate.StringWithTail(title, 20, ""))
	}

	// Limit to 30MiB songs for now
	if ssize > 31457280 {
		return errors.New("song too big")
	}

	tmpFile, err := os.Create(tmpFileName)
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	err = idx.Fetch(ctx, id, tmpFile)
	if err != nil {
		return err
	}

	kind, err := filetype.MatchFile(tmpFileName)
	if err != nil {
		return err
	}
	if kind.MIME.Value == "" {
		return fmt.Errorf("mime type not found. damaged file?")
	}

	song, err := os.Open(tmpFileName)
	if err != nil {
		return err
	}
	defer song.Close()

	if fetchMetadata {
		s.Suffix = " 🌍 fetching metadata..."
		err := fixMetadata(id, tmpFileName, meta)
		if err != nil {
			meta["metadata source"] = []byte("🤷")
		}
	}

	s.Stop()

	// Sort metadata
	keys := []string{
		"title", "artist", "album", "genre", "year", "metadata source", "filename", "_id",
	}
	for _, k := range keys {
		printMetadata(k, meta[k], headerColor)
	}

	song.Seek(0, 0)
	return play(ctx, kind.MIME.Value, song)
}

func fixMetadata(id, song string, meta map[string][]byte) error {
	fprinter := fps.New(filepath.Join(defaultIndexDir(), "acoustid.db"))

	fmeta, err := fprinter.Fingerprint(id, song)
	if err != nil {
		return err
	}

	if string(meta["artist"]) == "" || overrideMetadata {
		meta["artist"] = []byte(fmeta.Artist)
	}
	if string(meta["title"]) == "" || overrideMetadata {
		meta["title"] = []byte(fmeta.Title)
	}
	if string(meta["album"]) == "" || overrideMetadata {
		meta["album"] = []byte(fmeta.Album)
	}

	meta["metadata source"] = []byte(fmt.Sprintf("%t", fmeta.Cached))

	return nil
}
