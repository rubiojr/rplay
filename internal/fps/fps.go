package fps

import (
	"errors"

	"github.com/asdine/storm"
	"github.com/rubiojr/rplay/internal/acoustid"
)

var ErrMetadataNotFound = errors.New("metadata not found")

type Fingerprinter interface {
	Fingerprint(id, path string) (*Metadata, error)
}

type AcoustIDFingerprinter struct {
	dbPath string
}

type Metadata struct {
	FileID string `json:"fileid" storm:"id"`
	Title  string `json:"title"`
	Album  string `json:"album"`
	Artist string `json:"artist"`
	Cached bool   `json:"cached"`
}

func New(dbPath string) Fingerprinter {
	return &AcoustIDFingerprinter{dbPath: dbPath}
}

func (f *AcoustIDFingerprinter) Fingerprint(id, path string) (*Metadata, error) {
	var meta *Metadata
	if meta, err := f.metadataFromDB(id); err == nil {
		meta.Cached = true
		return meta, nil
	}

	fp, err := acoustid.NewFingerprint(path)
	if err != nil {
		return nil, err
	}

	resp, err := acoustid.MakeAcoustIDRequest(fp)
	if err != nil {
		return nil, err
	}

	if len(resp.Results) == 0 {
		return nil, ErrMetadataNotFound
	}

	result := resp.Results[0]

	if len(result.Recordings) == 0 {
		return nil, ErrMetadataNotFound
	}

	rec := result.Recordings[0]

	meta = &Metadata{}
	meta.FileID = id
	meta.Cached = false
	if len(rec.Artists) > 0 {
		meta.Artist = rec.Artists[0].Name
	}
	if len(rec.ReleaseGroups) > 0 {
		meta.Album = rec.ReleaseGroups[0].Title
	}
	meta.Title = rec.Title

	db, err := storm.Open(f.dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	err = db.Save(meta)
	if err != nil {
		panic(err)
	}

	return meta, nil
}

func (f *AcoustIDFingerprinter) metadataFromDB(id string) (*Metadata, error) {
	db, err := storm.Open(f.dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	meta := &Metadata{}
	err = db.One("FileID", id, meta)
	if err != nil {
		return nil, err
	}

	return meta, nil
}
