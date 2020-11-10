package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/blugelabs/bluge"
	"github.com/briandowns/spinner"
	"github.com/dhowden/tag"
	"github.com/muesli/reflow/padding"
	"github.com/muesli/reflow/truncate"
	"github.com/rubiojr/rapi/repository"
	"github.com/rubiojr/rapi/restic"
	"github.com/rubiojr/rindex"
	"github.com/rubiojr/rindex/blugeindex"
	"github.com/urfave/cli/v2"
)

var tStart = time.Now()

const statusStrLen = 30

func init() {
	cmd := &cli.Command{
		Name:   "index",
		Usage:  "Index the repository",
		Action: indexRepo,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:     "log-errors",
				Usage:    "Log errors",
				Required: false,
			},
		},
	}
	appCommands = append(appCommands, cmd)
}

func indexRepo(cli *cli.Context) error {
	progress := make(chan rindex.IndexStats, 10)
	idxOpts := &rindex.IndexOptions{
		Filter:    "*.mp3",
		IndexPath: cli.String("index-path"),
	}
	go progressMonitor(progress)

	stats, err := rindex.CustomIndex(idxOpts, &MP3Indexer{}, progress)
	if err != nil {
		panic(err)
	}
	fmt.Printf(
		"\n💥 %d indexed, %d already present. Took %d seconds.\n",
		stats.IndexedNodes,
		stats.AlreadyIndexed,
		int(time.Since(tStart).Seconds()),
	)
	return nil
}

type MP3Indexer struct{}

func (i *MP3Indexer) ShouldIndex(fileID string, bindex *blugeindex.BlugeIndex, node *restic.Node, repo *repository.Repository) (*bluge.Document, bool) {
	repoId := repo.Config().ID
	repoLocation := repo.Backend().Location()
	buf, err := repo.LoadBlob(context.Background(), restic.DataBlob, node.Content[0], nil)
	var id3Info tag.Metadata
	if err == nil {
		id3Info, err = tag.ReadFrom(bytes.NewReader(buf))
	}

	artist := ""
	title := ""
	album := ""
	genre := ""
	year := 0
	if id3Info != nil {
		artist = id3Info.Artist()
		title = id3Info.Title()
		album = id3Info.Album()
		genre = id3Info.Genre()
		year = id3Info.Year()
	}
	doc := bluge.NewDocument(fileID).
		AddField(bluge.NewTextField("filename", node.Name).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("blobs", blobsToString(node.Content)).StoreValue()).
		AddField(bluge.NewTextField("artist", artist).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("title", title).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("album", album).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("genre", genre).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("year", strconv.Itoa(year)).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("repository_location", repoLocation).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("repository_id", repoId).StoreValue().HighlightMatches())
	return doc, true
}

func progressMonitor(progress chan rindex.IndexStats) {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Color("fgGreen")
	s.Suffix = " Analyzing the repository..."
	for {
		select {
		case p := <-progress:
			lm := p.LastMatch
			if lm == "" {
				lm = "Searching for MP3 files..."
			}
			ls := truncate.StringWithTail(lm, statusStrLen, "...")
			rate := float64(p.ScannedNodes*1000000000) / float64(time.Since(tStart))
			s.Suffix = fmt.Sprintf(
				" %s 🎯 %d new, %d skipped, %d errors, %.0f f/s, %d files scanned",
				padding.String(ls, statusStrLen),
				p.IndexedNodes,
				p.AlreadyIndexed,
				len(p.Errors),
				rate,
				p.ScannedNodes,
			)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	s.Stop()
}

func blobsToString(ids restic.IDs) string {
	j, err := json.Marshal(ids)
	if err != nil {
		panic(err)
	}
	return string(j)
}
