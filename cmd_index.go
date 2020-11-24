package main

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
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

var audioRegexp *regexp.Regexp

type AudioFileMatcher struct{}

type MP3DocumentBuilder struct{}

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

func (m *AudioFileMatcher) ShouldIndex(path string) bool {
	return audioRegexp.Match([]byte(path))
}

func indexRepo(cli *cli.Context) error {
	var err error

	audioRegexp, err = regexp.Compile("\\.(flac|ogg|mp3)$")
	if err != nil {
		return err
	}

	progress := make(chan rindex.IndexStats, 10)
	idxOpts := rindex.IndexOptions{
		FileMatcher:     &AudioFileMatcher{},
		AppendFileMeta:  true,
		DocumentBuilder: &MP3DocumentBuilder{},
	}
	go progressMonitor(cli.Bool("log-errors"), progress)

	idx, err := rindex.New(indexPath)
	if err != nil {
		return err
	}
	stats, err := idx.Index(context.Background(), idxOpts, progress)
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

func (i MP3DocumentBuilder) BuildDocument(fileID string, bindex blugeindex.BlugeIndex, node *restic.Node, repo *repository.Repository) *bluge.Document {
	buf, err := repo.LoadBlob(context.Background(), restic.DataBlob, node.Content[0], nil)
	var id3Info tag.Metadata
	if err == nil {
		// ignore errors when reading tags, we still want to index them
		id3Info, _ = tag.ReadFrom(bytes.NewReader(buf))
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
		AddField(bluge.NewTextField("artist", artist).StoreValue()).
		AddField(bluge.NewTextField("title", title).StoreValue()).
		AddField(bluge.NewTextField("album", album).StoreValue()).
		AddField(bluge.NewTextField("genre", genre).StoreValue()).
		AddField(bluge.NewNumericField("year", float64(year)).StoreValue())
	return doc
}

func progressMonitor(logErrors bool, progress chan rindex.IndexStats) {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Color("fgGreen")
	s.Suffix = " Analyzing the repository..."
	lastError := ""
	for {
		select {
		case p := <-progress:
			if logErrors {
				if len(p.Errors) > 0 {
					e := p.Errors[len(p.Errors)-1].Error()
					if e != lastError {
						panic(e)
						fmt.Println("\n", e)
						lastError = e
					}
				}
			}
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
				p.ScannedFiles,
			)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	s.Stop()
}
