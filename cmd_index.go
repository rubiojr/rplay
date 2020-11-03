package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blugelabs/bluge"
	"github.com/dhowden/tag"
	"github.com/muesli/reflow/ansi"
	"github.com/muesli/reflow/padding"
	"github.com/rubiojr/rapi"
	"github.com/rubiojr/rapi/repository"
	"github.com/rubiojr/rapi/restic"
	"github.com/schollz/spinner"
	"github.com/urfave/cli/v2"
)

type fileInfo struct {
	blobIDs      restic.IDs
	path         string
	name         string
	fid          string
	id3Info      tag.Metadata
	repoId       string
	repoLocation string
}

// fileID is a 256-bit hash that distinguishes unique files.
type fileID [32]byte

var uniqueFiles = map[string]fileInfo{}

var scannedBlobs = 0
var indexedFiles = 0
var totalBlobs = 0
var lastScanned = "Waiting for MP3s to be found..."
var alreadyIndexed = 0
var indexLoaded = false
var blugeReader *bluge.Reader
var metadataErrors = 0
var tStart = time.Now()

const statusStrLen = 25

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
	go progressMonitor()
	repo, err := rapi.OpenRepository(globalOptions)
	if err != nil {
		return err
	}
	ctx := context.Background()

	if err = repo.LoadIndex(ctx); err != nil {
		return err
	}

	idx := repo.Index()
	treeBlobs := []restic.ID{}
	for blob := range idx.Each(ctx) {
		if blob.Type == restic.TreeBlob {
			treeBlobs = append(treeBlobs, blob.ID)
		}
	}

	totalBlobs = len(treeBlobs)
	indexLoaded = true

	for _, blob := range treeBlobs {
		repo.LoadBlob(ctx, restic.TreeBlob, blob, nil)
		tree, err := repo.LoadTree(ctx, blob)
		if err != nil {
			return err
		}

		for _, node := range tree.Nodes {
			if node.Type != "file" {
				continue
			}
			err := indexFile(blob, node, repo)
			if err != nil && cli.Bool("log-errors") {
				fmt.Fprintf(os.Stderr, "error indexing tree blob %s [%s]: %v\n", node.Name, blob, err)
			}
		}
		scannedBlobs += 1
	}

	fmt.Printf(
		"\n💥 %d indexed, %d already present. Took %d seconds.\n",
		indexedFiles,
		alreadyIndexed,
		int(time.Since(tStart).Seconds()),
	)
	return nil
}

func wasIndexed(id string) (bool, error) {
	if firstTimeIndex {
		return false, nil
	}
	var err error
	if blugeReader == nil {
		blugeReader, err = bluge.OpenReader(blugeConf)
		if err != nil {
			panic(err)
		}
	}

	query := bluge.NewWildcardQuery(id).SetField("_id")
	request := bluge.NewAllMatches(query)

	documentMatchIterator, err := blugeReader.Search(context.Background(), request)
	if err != nil {
		panic(err)
	}

	match, err := documentMatchIterator.Next()
	if err == nil && match != nil {
		return true, nil
	}

	return false, nil
}

func indexFile(id restic.ID, node *restic.Node, repo *repository.Repository) error {
	if node == nil {
		return fmt.Errorf("nil node found in tree %s", id)
	}

	fid := fmt.Sprintf("%x", makeFileIDByContents(node))
	if _, ok := uniqueFiles[fid]; ok {
		return nil
	}

	match, err := filepath.Match("*.mp3", strings.ToLower(node.Name))
	if err != nil {
		return err
	}

	if match {
		lastScanned = node.Name
		meta := fileInfo{
			blobIDs:      node.Content,
			path:         node.Path,
			name:         node.Name,
			fid:          fid,
			repoId:       repo.Config().ID,
			repoLocation: globalOptions.Repo,
		}
		uniqueFiles[fid] = meta
		if ok, _ := wasIndexed(fid); ok {
			alreadyIndexed += 1
			return nil
		}

		buf, err := repo.LoadBlob(context.Background(), restic.DataBlob, node.Content[0], nil)
		if err != nil {
			return err
		}
		meta.id3Info, err = tag.ReadFrom(bytes.NewReader(buf))
		if err != nil {
			metadataErrors += 1
			return fmt.Errorf("error reading ID3 tags from %s (%s)", node.Name, id)
		}
		return addToIndex(meta)
	}

	return nil
}

func makeFileIDByContents(node *restic.Node) fileID {
	var bb []byte
	for _, c := range node.Content {
		bb = append(bb, []byte(c[:])...)
	}
	return sha256.Sum256(bb)
}

func progressMonitor() {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Color("fgGreen")
	s.Suffix = " Analyzing the repository..."
	for {
		time.Sleep(100 * time.Millisecond)
		if indexLoaded {
			break
		}
	}
	remaining := ""
	for {
		ls := ansi.TruncateWithTail(lastScanned, statusStrLen, "...")
		rate := float64(scannedBlobs*1000000000) / float64(time.Now().Sub(tStart))
		remainingSec := (float64(totalBlobs-scannedBlobs) / rate)
		if remainingSec < 3600 {
			remaining = fmt.Sprintf("%.2f minutes", remainingSec/60)
		} else {
			remaining = fmt.Sprintf("%.2f hours", remainingSec/3600)
		}
		s.Suffix = fmt.Sprintf(
			" %s 🎯 %d new, %d skipped, %d errors, %.0f f/s, %s left",
			padding.String(ls, statusStrLen),
			indexedFiles,
			alreadyIndexed,
			metadataErrors,
			rate,
			remaining,
		)
		if scannedBlobs >= totalBlobs {
			s.Stop()
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func addToIndex(info fileInfo) error {
	blugeWriter, err := bluge.OpenWriter(blugeConf)
	if err != nil {
		return err
	}
	defer blugeWriter.Close()
	var artist, title, album, genre string
	var year int
	if info.id3Info != nil {
		artist = info.id3Info.Artist()
		title = info.id3Info.Title()
		album = info.id3Info.Album()
		genre = info.id3Info.Genre()
		year = info.id3Info.Year()
	} else {
		artist = ""
		title = ""
		album = ""
		genre = ""
		year = 0
	}
	doc := bluge.NewDocument(info.fid).
		AddField(bluge.NewTextField("filename", info.name).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("blobs", blobsToString(info.blobIDs)).StoreValue()).
		AddField(bluge.NewTextField("artist", artist).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("title", title).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("album", album).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("genre", genre).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("year", strconv.Itoa(year)).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("repository_location", info.repoLocation).StoreValue().HighlightMatches()).
		AddField(bluge.NewTextField("repository_id", info.repoId).StoreValue().HighlightMatches())

	err = blugeWriter.Update(doc.ID(), doc)
	if err == nil {
		indexedFiles += 1
	}
	return err
}

func blobsToString(ids restic.IDs) string {
	j, err := json.Marshal(ids)
	if err != nil {
		panic(err)
	}
	return string(j)
}
